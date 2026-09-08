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
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
)

const behaviorCaptchaTTL = 5 * time.Minute

type behaviorCaptchaChallenge struct {
	X       int       `json:"x"`
	Y       int       `json:"y"`
	DX      int       `json:"dx"`
	DY      int       `json:"dy"`
	Created time.Time `json:"created"`
}

var (
	behaviorCaptchaBuilder slide.Builder
	behaviorCaptchaOnce    sync.Once
	behaviorCaptchaLocal   sync.Map
)

func initBehaviorCaptcha() {
	behaviorCaptchaOnce.Do(func() {
		backgrounds, err := imagesv2.GetImages()
		if err != nil {
			panic("load behavior captcha backgrounds: " + err.Error())
		}
		assetTiles, err := tiles.GetTiles()
		if err != nil {
			panic("load behavior captcha tiles: " + err.Error())
		}
		graphs := make([]*slide.GraphImage, 0, len(assetTiles))
		for _, tile := range assetTiles {
			graphs = append(graphs, &slide.GraphImage{
				OverlayImage: tile.OverlayImage,
				ShadowImage:  tile.ShadowImage,
				MaskImage:    tile.MaskImage,
			})
		}
		behaviorCaptchaBuilder = slide.NewBuilder(
			slide.WithImageSize(option.Size{Width: 320, Height: 160}),
			slide.WithRangeGraphSize(option.RangeVal{Min: 40, Max: 50}),
			slide.WithGenGraphNumber(1),
		)
		behaviorCaptchaBuilder.SetResources(
			slide.WithBackgrounds(backgrounds),
			slide.WithGraphImages(graphs),
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
	captcha, err := behaviorCaptchaBuilder.MakeDragDrop().Generate()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	block := captcha.GetData()
	if block == nil {
		common.ApiError(c, errors.New("behavior captcha generated without block data"))
		return
	}
	id, err := newBehaviorCaptchaID()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := storeBehaviorCaptcha(id, behaviorCaptchaChallenge{
		X: block.X, Y: block.Y, DX: block.DX, DY: block.DY, Created: time.Now(),
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	master, err := captcha.GetMasterImage().ToBase64()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tile, err := captcha.GetTileImage().ToBase64()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id": id, "master_image": master, "tile_image": tile,
		"width": 320, "height": 160, "tile_width": block.Width, "tile_height": block.Height,
		"tile_start_y": block.DY,
	})
}

func verifyBehaviorCaptcha(id string, x, y int) bool {
	if id == "" || x < 0 || y < 0 {
		return false
	}
	challenge, ok := takeBehaviorCaptcha(id)
	if !ok {
		return false
	}
	return slide.Validate(challenge.X, challenge.Y, x, y, 5)
}

func VerifyBehaviorCaptcha(id string, x, y int) bool {
	return verifyBehaviorCaptcha(id, x, y)
}

func behaviorCaptchaError(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": "请完成拖动验证码",
		"code":    "BEHAVIOR_CAPTCHA_REQUIRED",
	})
}

func parseCaptchaCoordinate(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}
