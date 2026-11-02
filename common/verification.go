package common

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

func EffectiveVerificationValidMinutes() int {
	if VerificationValidMinutes <= 0 {
		return 10
	}
	if VerificationValidMinutes > 15 {
		return 15
	}
	return VerificationValidMinutes
}

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func verificationRedisKey(key string, purpose string) string {
	return "verification:" + verificationDigest("key:"+purpose+"\x00"+key)
}

func verificationCodeDigest(code string) string {
	return verificationDigest("code:" + code)
}

func verificationDigest(value string) string {
	mac := hmac.New(sha256.New, []byte(SessionSecret))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func consumeVerificationCodeFromRedis(client *redis.Client, key string, code string, purpose string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const consumeScript = `
local stored = redis.call('GET', KEYS[1])
if stored and stored == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`
	consumed, err := client.Eval(ctx, consumeScript, []string{verificationRedisKey(key, purpose)}, verificationCodeDigest(code)).Int64()
	return err == nil && consumed == 1
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) error {
	if RedisEnabled && RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := RDB.Set(ctx, verificationRedisKey(key, purpose), verificationCodeDigest(code), time.Duration(EffectiveVerificationValidMinutes())*time.Minute).Err(); err != nil {
			return fmt.Errorf("store verification code in redis: %w", err)
		}
		return nil
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[purpose+key] = verificationValue{
		code: verificationCodeDigest(code),
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
	return nil
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	if RedisEnabled && RDB != nil {
		return consumeVerificationCodeFromRedis(RDB, key, code, purpose)
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	mapKey := purpose + key
	value, okay := verificationMap[mapKey]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= EffectiveVerificationValidMinutes()*60 {
		delete(verificationMap, mapKey)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(value.code), []byte(verificationCodeDigest(code))) != 1 {
		return false
	}
	delete(verificationMap, mapKey)
	return true
}

func DeleteKey(key string, purpose string) {
	if RedisEnabled && RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := RDB.Del(ctx, verificationRedisKey(key, purpose)).Err(); err != nil {
			SysError("failed to delete verification code from redis: " + err.Error())
		}
		return
	}

	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, purpose+key)
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= EffectiveVerificationValidMinutes()*60 {
			delete(verificationMap, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
