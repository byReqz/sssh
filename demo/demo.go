package main

import (
	"fmt"
	gap "github.com/byReqz/go-ask-password"
	"github.com/byReqz/sssh"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
)

func main() {
	var (
		user         = "test"
		pass         = "test"
		fallbackfunc = func() (string, error) { return gap.AskPassword("Password: ") }
	)

	conf := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			ssh.PasswordCallback(fallbackfunc),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := sssh.Connect(os.Args[1]+":22", conf)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	err = sssh.StartInteractiveShell(client)
	if err != nil {
		log.Fatal(err)
	}

	err = sssh.CopyFile(client, "demo.go", "demo.go")
	if err != nil {
		log.Fatal(err)
	}

	buf, err := sssh.StartCommand(client, "cat demo.go")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(buf))

	err = sssh.RemoveFile(client, "demo.go")
	if err != nil {
		log.Fatal(err)
	}

	/*
		buf, err = sssh.StartInteractiveCommand(client, "nano")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(string(buf))
	*/
}
