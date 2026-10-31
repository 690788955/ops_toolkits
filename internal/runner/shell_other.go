//go:build !windows

package runner

// 非 Windows 平台不读注册表；bash 直接在 PATH 里。
func registryGitInstallDirs() []string {
	return nil
}