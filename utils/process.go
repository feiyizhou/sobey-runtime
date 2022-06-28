package util

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func Exec(name string, args []string, attr *syscall.SysProcAttr, inFile, outFile, errFile string) (string, error) {
	command := exec.Command(name, args...)
	if attr != nil {
		command.SysProcAttr = attr
	}
	if len(inFile) != 0 {
		in, err := openFile(inFile, "输入")
		if err != nil {
			return "", err
		}
		command.Stdin = in
	} else {
		command.Stdin = os.Stdin
	}
	if len(outFile) != 0 {
		out, err := openFile(outFile, "输出")
		if err != nil {
			return "", err
		}
		command.Stdout = out
	} else {
		command.Stdout = os.Stdout
	}
	if len(errFile) != 0 {
		er, err := openFile(errFile, "异常")
		if err != nil {
			return "", err
		}
		command.Stderr = er
	} else {
		command.Stderr = os.Stderr
	}
	err := command.Start()
	if err != nil {
		return "", err
	}
	err = command.Wait()
	if err != nil {
		return "", err
	}
	return strconv.Itoa(command.Process.Pid), err
}

func openFile(path, fileName string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Printf("打开%s文件错误：%v", fileName, err)
		return nil, err
	}
	return file, err
}
