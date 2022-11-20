package sssh

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"os"
	"strings"
)

// Connect opens a new ssh client.
//
// The difference to ssh.Dial is that every given auth method is tried individually till the connection is up or all have been tried.
func Connect(addr string, conf *ssh.ClientConfig) (*ssh.Client, error) {
	var (
		client *ssh.Client
		err    error
	)
	if len(conf.Auth) == 0 {
		return client, fmt.Errorf("Connect: no auth methods given")
	}

	for _, auth := range conf.Auth {
		c := conf
		c.Auth = []ssh.AuthMethod{auth}
		client, err = ssh.Dial("tcp", addr, c)
		if err == nil || !strings.Contains(err.Error(), "ssh: unable to authenticate") {
			break
		}
	}

	return client, err
}

// GetTermdata returns a set of values from the currently used terminal that is required to request a remote pty.
//
// Returns the terminal file descriptor, terminal type (will fall back to xterm if none is found), window width and window height.
// Will error when no real terminal is attached.
func GetTermdata() (int, string, int, int, error) {
	var (
		fd       int
		termtype string
		width    int
		height   int
		err      error
	)
	fd = int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fd, termtype, width, height, fmt.Errorf("GetTermdata: not attached to a real terminal")
	}

	termtype = os.Getenv("TERM")
	if termtype == "" {
		termtype = "xterm"
	}

	width, height, err = term.GetSize(fd)
	return fd, termtype, width, height, err
}

// StartCommand starts the given command non-interactively on the given ssh session. This is pretty much equal to session.CombinedOutput()
func StartCommand(client *ssh.Client, command string) ([]byte, error) {
	var buf []byte
	session, err := client.NewSession()
	if err != nil {
		return buf, err
	}
	defer func(session *ssh.Session) { _ = session.Close() }(session)

	buf, err = session.CombinedOutput(command)
	if err != nil {
		return buf, err
	}

	return buf, nil
}

/*
// StartInteractiveCommand starts a command in a pty on the given client.
func StartInteractiveCommand(client *ssh.Client, command string) ([]byte, error) {
	var buf []byte
	session, err := client.NewSession()
	if err != nil {
		return buf, err
	}
	defer func(session *ssh.Session) { _ = session.Close() }(session)

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	termfd, termtype, w, h, err := GetTermdata()
	if err != nil {
		return buf, err
	}

	err = session.RequestPty(termtype, h, w, modes)
	if err != nil {
		return buf, err
	}

	originalState, err := term.MakeRaw(termfd)
	if err != nil {
		return buf, err
	}
	defer func(fd int, oldState *term.State) { _ = term.Restore(fd, oldState) }(termfd, originalState)

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	buf, err = session.CombinedOutput(command)
	if err != nil {
		return buf, err
	}

	return buf, nil
}
*/

// StartInteractiveShell starts an interactive shell in its own pty on the given client. Returns a copy of stdin, stdout and stderr.
func StartInteractiveShell(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func(session *ssh.Session) { _ = session.Close() }(session)

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	termfd, termtype, w, h, err := GetTermdata()
	if err != nil {
		return err
	}

	err = session.RequestPty(termtype, h, w, modes)
	if err != nil {
		return err
	}

	originalState, err := term.MakeRaw(termfd)
	if err != nil {
		return err
	}
	defer func(fd int, oldState *term.State) { _ = term.Restore(fd, oldState) }(termfd, originalState)

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	err = session.Shell()
	if err != nil {
		return err
	}

	err = session.Wait()
	if err != nil {
		return err
	}
	return nil
}
