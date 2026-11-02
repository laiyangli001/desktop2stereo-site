package common

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetVerificationState(t *testing.T) {
	t.Helper()
	previousRedisEnabled := RedisEnabled
	previousRedisClient := RDB
	previousValidMinutes := VerificationValidMinutes
	verificationMutex.Lock()
	previousCodes := make(map[string]verificationValue, len(verificationMap))
	for key, value := range verificationMap {
		previousCodes[key] = value
	}
	verificationMap = make(map[string]verificationValue)
	verificationMutex.Unlock()

	t.Cleanup(func() {
		if RDB != nil && RDB != previousRedisClient {
			_ = RDB.Close()
		}
		RedisEnabled = previousRedisEnabled
		RDB = previousRedisClient
		VerificationValidMinutes = previousValidMinutes
		verificationMutex.Lock()
		verificationMap = previousCodes
		verificationMutex.Unlock()
	})
}

func TestVerificationCodeSharedAcrossRedisClientsAndExpires(t *testing.T) {
	resetVerificationState(t)
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = clientA.Close() })
	t.Cleanup(func() { _ = clientB.Close() })

	RedisEnabled = true
	RDB = clientA
	const email = "test-user@example.test"
	const code = "482913"
	require.NoError(t, RegisterVerificationCodeWithKey(email, code, EmailVerificationPurpose))

	redisKey := verificationRedisKey(email, EmailVerificationPurpose)
	assert.NotContains(t, redisKey, email)
	storedDigest, err := server.Get(redisKey)
	require.NoError(t, err)
	assert.NotEqual(t, code, storedDigest)
	assert.Greater(t, server.TTL(redisKey), time.Duration(0))
	assert.LessOrEqual(t, server.TTL(redisKey), 15*time.Minute)

	RDB = clientB
	assert.False(t, VerifyCodeWithKey(email, "000000", EmailVerificationPurpose))
	assert.True(t, VerifyCodeWithKey(email, code, EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(email, code, EmailVerificationPurpose))

	const expiringEmail = "expiring@example.test"
	require.NoError(t, RegisterVerificationCodeWithKey(expiringEmail, "528301", EmailVerificationPurpose))
	server.FastForward(time.Duration(EffectiveVerificationValidMinutes()) * time.Minute)
	assert.False(t, VerifyCodeWithKey(expiringEmail, "528301", EmailVerificationPurpose))
}

func TestVerificationCodeFallsBackToMemoryWithoutRedis(t *testing.T) {
	resetVerificationState(t)
	RedisEnabled = false
	const email = "single-instance@example.test"
	const code = "731604"

	require.NoError(t, RegisterVerificationCodeWithKey(email, code, EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(email, "000000", EmailVerificationPurpose))
	assert.True(t, VerifyCodeWithKey(email, code, EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(email, code, EmailVerificationPurpose))

	const expiringEmail = "memory-expiring@example.test"
	const expiringCode = "605918"
	require.NoError(t, RegisterVerificationCodeWithKey(expiringEmail, expiringCode, EmailVerificationPurpose))
	verificationMutex.Lock()
	value := verificationMap[EmailVerificationPurpose+expiringEmail]
	value.time = time.Now().Add(-time.Duration(EffectiveVerificationValidMinutes()) * time.Minute)
	verificationMap[EmailVerificationPurpose+expiringEmail] = value
	verificationMutex.Unlock()
	assert.False(t, VerifyCodeWithKey(expiringEmail, expiringCode, EmailVerificationPurpose))
}

func TestVerificationCodeRedisConsumptionIsAtomicAcrossClients(t *testing.T) {
	resetVerificationState(t)
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = clientA.Close() })
	t.Cleanup(func() { _ = clientB.Close() })
	RedisEnabled = true
	RDB = clientA
	const email = "atomic@example.test"
	const code = "829415"
	require.NoError(t, RegisterVerificationCodeWithKey(email, code, EmailVerificationPurpose))

	var winners atomic.Int32
	var wait sync.WaitGroup
	for index := range 20 {
		client := clientA
		if index%2 == 1 {
			client = clientB
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			if consumeVerificationCodeFromRedis(client, email, code, EmailVerificationPurpose) {
				winners.Add(1)
			}
		}()
	}
	wait.Wait()
	assert.EqualValues(t, 1, winners.Load())
	assert.False(t, VerifyCodeWithKey(email, code, EmailVerificationPurpose))
}

func TestVerificationCodeDoesNotFallbackWhenConfiguredRedisFails(t *testing.T) {
	resetVerificationState(t)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	RedisEnabled = false
	require.NoError(t, RegisterVerificationCodeWithKey("fail-closed@example.test", "194062", EmailVerificationPurpose))
	RedisEnabled = true
	RDB = client
	server.Close()

	err := RegisterVerificationCodeWithKey("new@example.test", "630815", EmailVerificationPurpose)
	assert.Error(t, err)
	assert.False(t, VerifyCodeWithKey("fail-closed@example.test", "194062", EmailVerificationPurpose))
}

func TestVerificationStorageKeysDoNotContainRecipient(t *testing.T) {
	key := verificationRedisKey("global.user+tag@example.test", PasswordResetPurpose)
	assert.True(t, strings.HasPrefix(key, "verification:"))
	assert.NotContains(t, key, "global.user")
	assert.NotContains(t, key, "example.test")
}
