package main

import (
	"github.com/go-ini/ini"
	"os/exec"
	"runtime"
)

func get_uname() string {
	var str string
	cmd := exec.Command("uname", "-r", "-m")
	res, err := cmd.CombinedOutput()
	if err != nil {
		str = ""
	} else {
		str = string(res)
	}
	return str
}

func get_runtime() (string, string) {
	return runtime.GOOS, runtime.GOARCH
}

func get_os_release() string {
	s := ""
	cfg, err := ini.Load("/etc/os-release")
	if err == nil {
		s = cfg.Section("").Key("NAME").String()
	}
	return s
}

func get_os_info() (string, string) {
	os, arch := get_runtime()
	uarch := get_uname()
	if uarch != "" {
		arch = uarch
	}
	if os == "linux" {
		los := get_os_release()
		if los != "" {
			os = los
		}
	}
	return os, arch
}
