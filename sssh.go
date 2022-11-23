package sssh

import (
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"io"
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

// StartInteractiveShell starts an interactive shell in its own pty on the given client.
func StartInteractiveShell(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func(session *ssh.Session) { _ = session.Close() }(session)

	modes := ssh.TerminalModes{
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

// WriteFile writes b to the target via sftp. Does not check if file is present at dst, will overwrite.
func WriteFile(client *ssh.Client, src []byte, dst string, fm os.FileMode) error {
	session, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer func(session *sftp.Client) { _ = session.Close() }(session)

	f, err := session.Create(dst)
	if err != nil {
		return fmt.Errorf("CopyFile: failed to create remote file: %s", err)
	}
	defer func(f *sftp.File) { _ = f.Close() }(f)
	_, err = f.Write(src)
	if err != nil {
		return fmt.Errorf("CopyFile: failed to write to remote file: %s", err)
	}
	err = f.Chmod(fm)
	if err != nil {
		return fmt.Errorf("CopyFile: failed to set permissions for remote file: %s", err)
	}
	_, err = session.Lstat(dst)
	if err != nil {
		return fmt.Errorf("CopyFile: file is absent after successful transfer: %s", err)
	}
	return nil
}

// CopyFile copies a file to the target via sftp. Does not check if file is present at dst, will overwrite.
func CopyFile(client *ssh.Client, src, dst string) error {
	stat, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("CopyFile: failed to stat source file: %s", err)
	}

	// a trailing / implies that the file should be placed into the given directory
	if strings.HasSuffix(dst, "/") {
		dst = dst + stat.Name()
	}

	srcfile, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("CopyFile: failed to read source file: %s", err)
	}
	return WriteFile(client, srcfile, dst, stat.Mode())
}

// ReadFile reads a remote file.
func ReadFile(client *ssh.Client, path string) ([]byte, os.FileInfo, error) {
	var (
		buf []byte
		fi  os.FileInfo
	)

	session, err := sftp.NewClient(client)
	if err != nil {
		return buf, fi, err
	}
	defer func(session *sftp.Client) { _ = session.Close() }(session)

	fi, err = session.Lstat(path)
	if err != nil {
		return buf, fi, fmt.Errorf("ReadFile: could not stat remote file: %s", err)
	}
	f, err := session.Open(path)
	if err != nil {
		return buf, fi, fmt.Errorf("ReadFile: could not open remote file: %s", err)
	}
	buf, err = io.ReadAll(f)
	if err != nil {
		return buf, fi, fmt.Errorf("ReadFile: could not read remote file: %s", err)
	}
	return buf, fi, nil
}

// PullFile copies a remote file to a local destination.
func PullFile(client *ssh.Client, src, dst string) error {
	file, fi, err := ReadFile(client, src)
	if err != nil {
		return fmt.Errorf("PullFile: could not get remote file: %s", err)
	}

	// a trailing / implies that the file should be placed into the given directory
	if strings.HasSuffix(dst, "/") {
		dst = dst + fi.Name()
	}

	err = os.WriteFile(dst, file, fi.Mode())
	if err != nil {
		return fmt.Errorf("PullFile: failed to write local file: %s", err)
	}
	return nil
}

// RemoveFile removes a file or (empty) directory via sftp.
func RemoveFile(client *ssh.Client, path string) error {
	session, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer func(session *sftp.Client) { _ = session.Close() }(session)

	err = session.Remove(path)
	if err != nil {
		return fmt.Errorf("RemoveFile: failed to remove remote file: %s", err)
	}

	_, err = session.Lstat(path)
	if err == nil {
		return fmt.Errorf("RemoveFile: file is still present after successful removal")
	}
	return nil
}

// MoveFile moves a file via sftp.
func MoveFile(client *ssh.Client, src, dst string) error {
	session, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer func(session *sftp.Client) { _ = session.Close() }(session)

	err = session.Rename(src, dst)
	if err != nil {
		return fmt.Errorf("MoveFile: failed to move remote file: %s", err)
	}

	_, err = session.Lstat(src)
	if err == nil {
		return fmt.Errorf("MoveFile: old file is still present after successful move")
	}
	return nil
}
