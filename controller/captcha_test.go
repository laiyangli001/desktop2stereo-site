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
	captcha, err := behaviorCaptchaBuilder.Make().Generate()
	require.NoError(t, err)
	require.NotNil(t, captcha.GetData())
	master, err := captcha.GetMasterImage().ToBase64()
	require.NoError(t, err)
	thumb, err := captcha.GetThumbImage().ToBase64()
	require.NoError(t, err)
	require.NotEmpty(t, master)
	require.NotEmpty(t, thumb)
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
		Targets: []behaviorCaptchaTarget{{X: 100, Y: 40, Width: 30, Height: 30}},
		Created: time.Now(),
	}))
	clicks := []CaptchaClick{{X: 110, Y: 50}}
	require.True(t, verifyBehaviorCaptcha(id, clicks))
	require.False(t, verifyBehaviorCaptcha(id, clicks))
}
