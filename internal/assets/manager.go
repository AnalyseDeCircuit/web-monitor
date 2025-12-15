package assets

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	// assetHashes 存储文件路径到哈希的映射
	// Key: 原始请求路径 (e.g., "/static/js/app.js")
	// Value: 哈希值 (e.g., "a1b2c3d4")
	assetHashes = make(map[string]string)
	mutex       sync.RWMutex
	staticDir   = "./static"
)

// Init 初始化静态资源管理器，计算所有静态文件的哈希
func Init() error {
	mutex.Lock()
	defer mutex.Unlock()

	return filepath.Walk(staticDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// 计算相对路径，作为 key
		// path: "static/js/app.js" -> rel: "js/app.js"
		rel, err := filepath.Rel(staticDir, path)
		if err != nil {
			return err
		}

		// 统一使用 forward slash
		rel = filepath.ToSlash(rel)
		key := "/static/" + rel

		hash, err := computeFileHash(path)
		if err != nil {
			return fmt.Errorf("failed to compute hash for %s: %v", path, err)
		}

		assetHashes[key] = hash
		return nil
	})
}

func computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil))[:8], nil // 使用前8位
}

// GetHashedPath 返回带有哈希的路径
// 输入: "/static/js/app.js"
// 输出: "/static-hashed/a1b2c3d4/js/app.js" (如果找到哈希)
//
//	"/static/js/app.js" (如果未找到)
func GetHashedPath(path string) string {
	mutex.RLock()
	hash, ok := assetHashes[path]
	mutex.RUnlock()

	if !ok {
		return path
	}

	// 构造新的路径格式: /static-hashed/{hash}/{original_rel_path}
	// path: /static/js/app.js
	// rel: js/app.js
	rel := strings.TrimPrefix(path, "/static/")
	return fmt.Sprintf("/static-hashed/%s/%s", hash, rel)
}
