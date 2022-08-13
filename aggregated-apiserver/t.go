package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {

	args := []string{"--kubeconfig", "./resources/config_local", "--insecure-skip-tls-verify=true"}

	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalln(err)
	}

}
