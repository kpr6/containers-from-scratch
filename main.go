package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// go run main.go run <cmd> <args>
func main() {
	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child()
	default:
		panic("help")
	}
}

func run() {
	fmt.Printf("Running %v \n", os.Args[2:])

	// /proc/self/exe is link to current process
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// CLONE_NEWUTS is to create a new UTS namespace, process will have a separate hostname
	// CLONE_NEWPID to create a new PID namespace, child processes inside container won't appear outside
	// CLONE_NEWNS is mount namespace, keeps file mounts of a container private
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	must(cmd.Run())
}

func child() {
	fmt.Printf("Running %v \n", os.Args[2:])

	cg()

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(syscall.Sethostname([]byte("container")))

	// set its own root fs
	must(syscall.Chroot("/home/liz/ubuntufs"))
	must(os.Chdir("/"))

	// proc is a pseudo fs (stored in-memory) exposed by kernel to userspace, mounting it to container
	must(syscall.Mount("proc", "proc", "proc", 0, ""))
	// example tmpfs mount to prove these mounts wont appear outside of container
	must(syscall.Mount("thing", "mytemp", "tmpfs", 0, ""))

	must(cmd.Run())

	// detach after container process exits
	must(syscall.Unmount("proc", 0))
	must(syscall.Unmount("thing", 0))
}

func cg() {
	// cgroups control access to resources like cpu, mem, pids etc for a collection of process
	cgroups := "/sys/fs/cgroup/"

	// /sys/fs/cgroup/memory/cgroups.procs is the default which hosts system uses
	mem := filepath.Join(cgroups, "memory")
	// create private cgroup inside memory to control only our containers memory
	must(os.Mkdir(filepath.Join(mem, "pranaye"), 0755))
	must(ioutil.WriteFile(filepath.Join(mem, "pranaye/memory.limit_in_bytes"), []byte("99424"), 0700))
	// swap mem
	must(ioutil.WriteFile(filepath.Join(mem, "pranaye/memory.memsw.limit_in_bytes"), []byte("99424"), 0700))
	// if there are no more processes running inside the container, cgroup will be deleted
	must(ioutil.WriteFile(filepath.Join(mem, "pranaye/notify_on_release"), []byte("1"), 0700))
	pid := strconv.Itoa(os.Getpid())
	// this is where it all starts, link forms here when current process id is written to cgroups.procs
	// which has info of all running processes inside the cgroup
	must(ioutil.WriteFile(filepath.Join(mem, "pranaye/cgroup.procs"), []byte(pid), 0700))
	// takes care of pid control

	pids := filepath.Join(cgroups, "pids")
	must(os.Mkdir(filepath.Join(pids, "liz"), 0755))
	must(ioutil.WriteFile(filepath.Join(pids, "liz/pids.max"), []byte("20"), 0700))
	// Removes the new cgroup in place after the container exits
	must(ioutil.WriteFile(filepath.Join(pids, "liz/notify_on_release"), []byte("1"), 0700))
	must(ioutil.WriteFile(filepath.Join(pids, "liz/cgroup.procs"), []byte(strconv.Itoa(os.Getpid())), 0700))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
