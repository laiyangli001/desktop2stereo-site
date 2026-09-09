/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/golang/freetype/truetype"
	"github.com/wenlng/go-captcha-assets/resources/fonts/fzshengsksjw"
	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
)

const behaviorCaptchaTTL = 5 * time.Minute

type behaviorCaptchaChallenge struct {
	Targets []behaviorCaptchaTarget `json:"targets"`
	Created time.Time               `json:"created"`
}

type behaviorCaptchaTarget struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

var (
	behaviorCaptchaBuilder click.Builder
	behaviorCaptchaOnce    sync.Once
	behaviorCaptchaLocal   sync.Map
)

func initBehaviorCaptcha() {
	behaviorCaptchaOnce.Do(func() {
		backgrounds, err := imagesv2.GetImages()
		if err != nil {
			panic("load behavior captcha backgrounds: " + err.Error())
		}
		font, err := fzshengsksjw.GetFont()
		if err != nil {
			panic("load behavior captcha font: " + err.Error())
		}
		behaviorCaptchaBuilder = click.NewBuilder(
			click.WithImageSize(option.Size{Width: 320, Height: 160}),
			click.WithRangeLen(option.RangeVal{Min: 6, Max: 7}),
			click.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 3}),
			click.WithRangeSize(option.RangeVal{Min: 24, Max: 30}),
		)
		behaviorCaptchaBuilder.SetResources(
			click.WithFonts([]*truetype.Font{font}),
			click.WithBackgrounds(backgrounds),
		)
	})
}

func newBehaviorCaptchaID() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func storeBehaviorCaptcha(id string, challenge behaviorCaptchaChallenge) error {
	data, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	if common.RedisEnabled && common.RDB != nil {
		return common.RDB.Set(context.Background(), "captcha:behavior:"+id, data, behaviorCaptchaTTL).Err()
	}
	behaviorCaptchaLocal.Store(id, challenge)
	return nil
}

func takeBehaviorCaptcha(id string) (behaviorCaptchaChallenge, bool) {
	if common.RedisEnabled && common.RDB != nil {
		key := "captcha:behavior:" + id
		data, err := common.RDB.GetDel(context.Background(), key).Bytes()
		if err != nil {
			return behaviorCaptchaChallenge{}, false
		}
		var challenge behaviorCaptchaChallenge
		if json.Unmarshal(data, &challenge) != nil || time.Since(challenge.Created) > behaviorCaptchaTTL {
			return behaviorCaptchaChallenge{}, false
		}
		return challenge, true
	}
	value, ok := behaviorCaptchaLocal.LoadAndDelete(id)
	if !ok {
		return behaviorCaptchaChallenge{}, false
	}
	challenge := value.(behaviorCaptchaChallenge)
	return challenge, time.Since(challenge.Created) <= behaviorCaptchaTTL
}

func GenerateBehaviorCaptcha(c *gin.Context) {
	initBehaviorCaptcha()
	captcha, err := behaviorCaptchaBuilder.Make().Generate()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	dots := captcha.GetData()
	if len(dots) == 0 {
		common.ApiError(c, errors.New("behavior captcha generated without click targets"))
		return
	}
	id, err := newBehaviorCaptchaID()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	targets := make([]behaviorCaptchaTarget, 0, len(dots))
	for _, dot := range dots {
		if dot == nil {
			continue
		}
		targets = append(targets, behaviorCaptchaTarget{
			X: dot.X, Y: dot.Y, Width: dot.Width, Height: dot.Height,
		})
	}
	if len(targets) == 0 {
		common.ApiError(c, errors.New("behavior captcha generated without usable click targets"))
		return
	}
	if err := storeBehaviorCaptcha(id, behaviorCaptchaChallenge{
		Targets: targets, Created: time.Now(),
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	master, err := captcha.GetMasterImage().ToBase64()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	thumb, err := captcha.GetThumbImage().ToBase64()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id": id, "master_image": master, "thumb_image": thumb,
		"width": 320, "height": 160, "required_clicks": len(targets),
	})
}

type CaptchaClick struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func verifyBehaviorCaptcha(id string, clicks []CaptchaClick) bool {
	if id == "" || len(clicks) == 0 {
		return false
	}
	challenge, ok := takeBehaviorCaptcha(id)
	if !ok {
		return false
	}
	if len(clicks) != len(challenge.Targets) {
		return false
	}
	matched := make([]bool, len(challenge.Targets))
	for _, point := range clicks {
		if point.X < 0 || point.Y < 0 {
			return false
		}
		found := false
		for index, target := range challenge.Targets {
			if !matched[index] && click.Validate(point.X, point.Y, target.X, target.Y, target.Width, target.Height, 5) {
				matched[index] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func VerifyBehaviorCaptcha(id string, clicks []CaptchaClick) bool {
	return verifyBehaviorCaptcha(id, clicks)
}

func behaviorCaptchaError(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": "请完成点击验证码",
		"code":    "BEHAVIOR_CAPTCHA_REQUIRED",
	})
}
