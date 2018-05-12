package utils

import (
    "bytes"
    "fmt"
    "syscall"
    "os/exec"
)

type ExecResut struct {
    ExitCode int
    Output   string
}

func Exec(name string, arg ...string) ExecResut {
    cmd := exec.Command(name, arg...)
    result := ExecResut{}

    var outb bytes.Buffer
    cmd.Stdout = &outb
    cmd.Stderr = &outb
    err := cmd.Run()

    result.Output = outb.String()
    if err != nil {
        if exitError, ok := err.(*exec.ExitError); ok {
            ws := exitError.Sys().(syscall.WaitStatus)
            result.ExitCode = ws.ExitStatus()
        } else {
            result.ExitCode = 128
            if result.Output == "" {
                result.Output = err.Error()
            }
        }
    } else {
        ws := cmd.ProcessState.Sys().(syscall.WaitStatus)
        result.ExitCode = ws.ExitStatus()
    }
    return result
}

func GitExec(repository string, command string, arg ...string) (ExecResut, error) {
    result := Exec("git", append([]string{"--git-dir", repository, command}, arg...)...)
    if result.ExitCode != 0 {
        return result, fmt.Errorf(result.Output)
    }

    return result, nil
}