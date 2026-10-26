package runner

import (
	"os/exec"
	"syscall"
)

func configureDeath(cmd *exec.Cmd) { cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL }
