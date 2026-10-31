//go:build windows

package runner

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// registryGitInstallDirs 读取 Git for Windows 写入注册表的安装目录。
// 只靠 PATH 找不到 bash：Git 安装时默认只把 cmd 目录加入 PATH，
// 而 bash.exe 位于 <InstallPath>\bin 下，所以必须问注册表才能覆盖装在 D 盘之类的情况。
func registryGitInstallDirs() []string {
	dirs := []string{}
	for _, keyPath := range []string{
		`SOFTWARE\GitForWindows`,
		`SOFTWARE\WOW6432Node\GitForWindows`,
	} {
		for _, access := range []uint32{
			registry.QUERY_VALUE | registry.WOW64_64KEY,
			registry.QUERY_VALUE | registry.WOW64_32KEY,
		} {
			key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, access)
			if err != nil {
				continue
			}
			value, _, valueErr := key.GetStringValue("InstallPath")
			_ = key.Close()
			if valueErr != nil {
				continue
			}
			if dir := strings.TrimSpace(value); dir != "" {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}