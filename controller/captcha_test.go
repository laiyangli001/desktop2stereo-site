/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestBehaviorCaptchaGeneratesImages(t *testing.T) {
	initBehaviorCaptcha()
	captcha, err := behaviorCaptchaBuilder.MakeDragDrop().Generate()
	require.NoError(t, err)
	require.NotNil(t, captcha.GetData())
	master, err := captcha.GetMasterImage().ToBase64()
	require.NoError(t, err)
	tile, err := captcha.GetTileImage().ToBase64()
	require.NoError(t, err)
	require.NotEmpty(t, master)
	require.NotEmpty(t, tile)
}

func TestBehaviorCaptchaIsOneTimeAndUsesRedisWhenAvailable(t *testing.T) {
	oldEnabled, oldRedis := common.RedisEnabled, common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = oldEnabled, oldRedis
	})

	id := "captcha-test-once"
	require.NoError(t, storeBehaviorCaptcha(id, behaviorCaptchaChallenge{
		X: 100, Y: 40, Created: time.Now(),
	}))
	require.True(t, verifyBehaviorCaptcha(id, 100, 40))
	require.False(t, verifyBehaviorCaptcha(id, 100, 40))
}
