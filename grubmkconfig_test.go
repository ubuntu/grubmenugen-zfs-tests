package main_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runGrubMkConfig setup and runs grubMkConfig.
func runGrubMkConfig(t *testing.T, env []string, testDir string) error {
	for _, path := range []string{
		"etc/grub.d/15_linux_zfs", "/etc/grub.d/00_header", "/etc/default/grub", "/usr/sbin/grub-mkconfig"} {
		copyFile(t, path, filepath.Join(testDir, path))
	}
	grubMkConfig := filepath.Join(testDir, "/usr/sbin/grub-mkconfig")
	updateMkConfig(t, grubMkConfig, testDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "fakeroot", grubMkConfig, "-o", filepath.Join(testDir, "grub.cfg"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	return cmd.Run()
}

// updateMkConfig updates sysconfigdir and exports variables in grub-mkconfig so that we target a specific
// /etc directory for grub scripts.
func updateMkConfig(t *testing.T, path, tmpdir string) {
	t.Helper()

	src, err := os.OpenFile(path, os.O_RDWR, 0755)
	if err != nil {
		t.Fatalf("can't open %q: %v", src.Name(), err)
	}
	defer src.Close()

	s := bufio.NewScanner(src)
	var text string
	for s.Scan() {
		t := s.Text()

		// We need to set grub_probe twice: once in environment (for subprocess) and once in grub_mkconfig directly
		t = strings.ReplaceAll(t, `sysconfdir="/etc"`, `sysconfdir="`+tmpdir+`/etc"`+
			"\nexport GRUB_LINUX_ZFS_TEST GRUB_LINUX_ZFS_TEST_INPUT GRUB_LINUX_ZFS_TEST_OUTPUT TEST_POOL_DIR TEST_MOKUTIL_SECUREBOOT TEST_MOCKZFS_CURRENT_ROOT_DATASET LC_ALL grub_probe\n")
		t = strings.ReplaceAll(t, `grub_probe="${sbindir}/grub-probe"`, "grub_probe=`which grub-probe`")

		if text == "" {
			text = t
		} else {
			text = text + "\n" + t
		}
	}
	if err := s.Err(); err != nil {
		t.Fatalf("can't replace sysconfigdir in %q: %v", path, err)
	}

	if err := src.Truncate(0); err != nil {
		t.Fatalf("can't truncate %q: %v", src.Name(), err)
	}
	if _, err := src.WriteAt([]byte(text), 0); err != nil {
		t.Fatalf("can't write to %q, %v", src.Name(), err)
	}
}
