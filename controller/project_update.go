package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	projectUpdateRepository = "laiyangli001/desktop2stereo-site"
	projectUpdateBranch     = "main"
	projectUpdateAPIURL     = "https://api.github.com/repos/" + projectUpdateRepository + "/commits/" + projectUpdateBranch
	projectUpdateScript     = "/usr/local/sbin/desktop2stereo-update"
)

var (
	projectUpdateSHAPattern  = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	projectUpdateMu          sync.Mutex
	projectUpdateAPIEndpoint = projectUpdateAPIURL
)

type projectUpdateConfig struct {
	Enabled      bool
	Script       string
	BackupScript string
	RequestFile  string
	StatusFile   string
}

type projectUpdateRuntimeStatus struct {
	State     string `json:"state"`
	Phase     string `json:"phase"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
	SHA       string `json:"sha,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type projectUpdateCommit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type projectUpdateRequest struct {
	SHA string `json:"sha"`
}

func getProjectUpdateConfig() projectUpdateConfig {
	script := strings.TrimSpace(os.Getenv("D2S_UPDATE_SCRIPT"))
	if script == "" {
		script = projectUpdateScript
	}
	backupScript := strings.TrimSpace(os.Getenv("D2S_UPDATE_BACKUP_SCRIPT"))
	if backupScript == "" {
		backupScript = "/usr/local/sbin/desktop2stereo-db-backup"
	}
	requestFile := strings.TrimSpace(os.Getenv("D2S_UPDATE_REQUEST_FILE"))
	statusFile := strings.TrimSpace(os.Getenv("D2S_UPDATE_STATUS_FILE"))
	if statusFile == "" && requestFile != "" {
		statusFile = filepath.Join(filepath.Dir(requestFile), "status.json")
	}
	return projectUpdateConfig{
		Enabled:      common.GetEnvOrDefaultBool("D2S_UPDATE_ENABLED", false),
		Script:       script,
		BackupScript: backupScript,
		RequestFile:  requestFile,
		StatusFile:   statusFile,
	}
}

func GetProjectUpdateStatus(c *gin.Context) {
	config := getProjectUpdateConfig()
	stat, statErr := os.Stat(config.Script)
	backupStat, backupStatErr := os.Stat(config.BackupScript)
	scriptReady := statErr == nil && stat.Mode().Perm()&0111 != 0
	backupReady := backupStatErr == nil && backupStat.Mode().Perm()&0111 != 0
	requestReady := config.RequestFile != "" && updateRequestDirectoryReady(config.RequestFile)
	runtimeStatus := readProjectUpdateRuntimeStatus(config.StatusFile)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"repository":  projectUpdateRepository,
			"branch":      projectUpdateBranch,
			"enabled":     config.Enabled,
			"configured":  config.Enabled && ((scriptReady && backupReady) || requestReady),
			"mode":        map[bool]string{true: "request-file", false: "script"}[config.RequestFile != ""],
			"script":      config.Script,
			"version":     common.Version,
			"current_sha": runtimeStatus.SHA,
			"runtime":     runtimeStatus,
		},
	})
}

func readProjectUpdateRuntimeStatus(filename string) projectUpdateRuntimeStatus {
	if filename == "" {
		return projectUpdateRuntimeStatus{State: "idle"}
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return projectUpdateRuntimeStatus{State: "idle"}
	}
	var status projectUpdateRuntimeStatus
	if err := json.Unmarshal(contents, &status); err != nil || status.State == "" {
		return projectUpdateRuntimeStatus{State: "idle"}
	}
	return status
}

func CheckProjectUpdate(c *gin.Context) {
	commit, err := fetchProjectUpdateCommit(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"repository": projectUpdateRepository,
			"branch":     projectUpdateBranch,
			"commit":     commit,
		},
	})
}

func ApplyProjectUpdate(c *gin.Context) {
	config := getProjectUpdateConfig()
	if !config.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "项目更新功能未启用"})
		return
	}
	if config.RequestFile == "" {
		stat, err := os.Stat(config.Script)
		backupStat, backupErr := os.Stat(config.BackupScript)
		if err != nil || stat.Mode().Perm()&0111 == 0 || backupErr != nil || backupStat.Mode().Perm()&0111 == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "服务器更新脚本或数据库备份脚本未正确安装"})
			return
		}
	} else if !updateRequestDirectoryReady(config.RequestFile) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "服务器更新请求目录未正确挂载"})
		return
	}
	if !projectUpdateMu.TryLock() {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "已有项目更新正在执行"})
		return
	}
	defer projectUpdateMu.Unlock()

	var request projectUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil || !projectUpdateSHAPattern.MatchString(request.SHA) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "必须提供有效的 40 位 commit SHA"})
		return
	}

	commit, err := fetchProjectUpdateCommit(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if !strings.EqualFold(request.SHA, commit.SHA) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "提交版本已变化，请重新检查更新"})
		return
	}

	sha := strings.ToLower(request.SHA)
	if config.RequestFile != "" {
		if err := queueProjectUpdateRequest(config.RequestFile, sha); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "无法写入服务器更新请求: " + err.Error()})
			return
		}
	} else {
		cmd := exec.Command(config.Script, sha)
		cmd.Dir = "/"
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "无法启动服务器更新脚本: " + err.Error()})
			return
		}
	}
	common.SysLog("project update started: repository=" + projectUpdateRepository + ", sha=" + sha)
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "项目更新请求已提交，服务将在备份、构建和健康检查完成后切换版本",
		"data":    gin.H{"sha": sha},
	})
}

func updateRequestDirectoryReady(requestFile string) bool {
	info, err := os.Stat(filepath.Dir(requestFile))
	return err == nil && info.IsDir()
}

func queueProjectUpdateRequest(requestFile string, sha string) error {
	temporary := requestFile + ".tmp"
	if err := os.WriteFile(temporary, []byte(sha+"\n"), 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, requestFile); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func fetchProjectUpdateCommit(parent context.Context) (projectUpdateCommit, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, projectUpdateAPIEndpoint, nil)
	if err != nil {
		return projectUpdateCommit{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "desktop2stereo-site-maintenance")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return projectUpdateCommit{}, fmt.Errorf("无法访问项目 GitHub 更新源: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return projectUpdateCommit{}, fmt.Errorf("GitHub 更新源返回 HTTP %d", response.StatusCode)
	}
	var commit projectUpdateCommit
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&commit); err != nil {
		return projectUpdateCommit{}, fmt.Errorf("解析 GitHub 更新信息失败: %w", err)
	}
	if !projectUpdateSHAPattern.MatchString(commit.SHA) {
		return projectUpdateCommit{}, errors.New("GitHub 更新源返回的 commit SHA 无效")
	}
	return commit, nil
}
