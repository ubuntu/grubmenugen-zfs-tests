package main_test

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var dangerous = flag.Bool("dangerous", false, "execute dangerous tests which may alter the system state")
var update = flag.Bool("update", false, "update golden files")

var zfsToBootlistTestDone = make(chan struct{}, 1)

func TestFromZFStoBootlist(t *testing.T) {
	defer close(zfsToBootlistTestDone)
	t.Parallel()
	zfsToBootlistTestDone <- struct{}{}

	ensureBinaryMocks(t)

	testCases := newTestCases(t)
	for name, tc := range testCases {
		tc := tc
		name := name
		t.Run(name, func(t *testing.T) {
			secureBootState := filepath.Base(filepath.Dir(tc.path))
			if secureBootState == "no-mokutil" {
				if !*dangerous {
					t.Skipf("don't run %q: dangerous is not set", name)
				}

				// remove mokutil from PATH
				if _, err := os.Stat("/usr/bin/mokutil"); os.IsExist(err) {
					if err := os.Rename("/usr/bin/mokutil", "/usr/bin/mokutil.bak"); err != nil {
						t.Fatal("couldn't rename mokutil to its backup", err)
					}
					defer os.Rename("/usr/bin/mokutil.bak", "/usr/bin/mokutil")
				}
			}

			testDir, cleanUp := tempDir(t)
			defer cleanUp()

			devices := newFakeDevices(t, filepath.Join(tc.path, "testcase.yaml"))
			systemRootDataset := devices.create(testDir, tc.fullTestName)

			out := filepath.Join(testDir, "bootlist")
			path := "PATH=mocks/zpool:mocks/zfs:mocks/date:" + os.Getenv("PATH")
			var securebootEnv string
			if secureBootState != "no-mokutil" {
				path = "PATH=mocks/mokutil:mocks/zpool:mocks/zfs:mocks/date:" + os.Getenv("PATH")
				securebootEnv = "TEST_MOKUTIL_SECUREBOOT=" + secureBootState
			}

			var mockZFSDatasetEnv string
			if systemRootDataset != "" {
				mockZFSDatasetEnv = "TEST_MOCKZFS_CURRENT_ROOT_DATASET=" + systemRootDataset
			}

			env := append(os.Environ(),
				path,
				"TEST_POOL_DIR="+testDir,
				"GRUB_LINUX_ZFS_TEST=bootlist",
				"GRUB_LINUX_ZFS_TEST_OUTPUT="+out,
				securebootEnv,
				mockZFSDatasetEnv)

			if err := runGrubMkConfig(t, env, testDir); err != nil {
				t.Fatal("got error, expected none", err)
			}

			reference := filepath.Join(tc.path, "bootlist")
			if *update {
				if err := ioutil.WriteFile(reference, []byte(anonymizeTempDirNames(t, out)), 0644); err != nil {
					t.Fatal("couldn't update reference file", err)
				}
			}

			assertFileContentAlmostEquals(t, out, reference, "generated and reference files are different.")
		})
	}
}

func TestMenuMetaData(t *testing.T) {
	t.Parallel()

	// Block until TestZfsToBootlist has fully ran. Timeout if not started
	select {
	case <-zfsToBootlistTestDone:
		// Testsuite has zfsToBootlist test running
		// Wait now for the channel to close
		<-zfsToBootlistTestDone
	// We waited for long enough for TestZfsToBootlist to start, it's probably filtered with -run
	case <-time.After(time.Second):
	}

	testCases := newTestCases(t)
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testDir, cleanUp := tempDir(t)
			defer cleanUp()

			out := getTempOrReferenceFile(t, *update,
				filepath.Join(testDir, "menumeta"),
				filepath.Join(tc.path, "menumeta"))
			env := append(os.Environ(),
				"GRUB_LINUX_ZFS_TEST=metamenu",
				"GRUB_LINUX_ZFS_TEST_INPUT="+filepath.Join(tc.path, "bootlist"),
				"GRUB_LINUX_ZFS_TEST_OUTPUT="+out)

			if err := runGrubMkConfig(t, env, testDir); err != nil {
				t.Fatal("got error, expected none", err)
			}

			assertFileContentAlmostEquals(t, out, filepath.Join(tc.path, "menumeta"), "generated and reference files are different.")
		})
	}
}

type TestCase struct {
	path         string
	fullTestName string
}

func newTestCases(t *testing.T) map[string]TestCase {
	testCases := make(map[string]TestCase)

	bootListsDir := "testdata/definitions"
	dirs, err := ioutil.ReadDir(bootListsDir)
	if err != nil {
		t.Fatal("couldn't read bootlists modes", err)
	}
	for _, d := range dirs {
		tcDirs, err := ioutil.ReadDir(filepath.Join(bootListsDir, d.Name()))
		if err != nil {
			t.Fatal("couldn't read bootlists test cases", err)
		}

		for _, tcd := range tcDirs {
			tcName := filepath.Join(d.Name(), tcd.Name())
			tcPath := filepath.Join(bootListsDir, tcName)
			if err != nil {
				t.Fatal("couldn't read test case", err)
			}

			testCases[tcName] = TestCase{
				path:         tcPath,
				fullTestName: strings.ReplaceAll(strings.Replace(tcPath, bootListsDir+"/", "", 1), "/", "_"),
			}
		}
	}

	return testCases
}
