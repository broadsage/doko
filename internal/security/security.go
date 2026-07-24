// Package security generates runtime security profiles (such as Seccomp and Landlock) based on the image's declared configuration.
package security

import (
	"encoding/json"
	"fmt"
)

// SeccompProfile represents a Linux Seccomp security profile.
type SeccompProfile struct {
	DefaultAction string        `json:"defaultAction"`
	Architectures []string      `json:"architectures"`
	Syscalls      []SyscallRule `json:"syscalls"`
}

// SyscallRule defines a set of syscalls with an associated action.
type SyscallRule struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}

// LandlockPolicy represents a Landlock LSM access restriction policy.
type LandlockPolicy struct {
	ReadablePaths   []string `json:"readable_paths"`
	WritablePaths   []string `json:"writable_paths"`
	ExecutablePaths []string `json:"executable_paths"`
}

// GenerateSeccompProfile builds a Seccomp JSON profile based on declared packages and ports.
func GenerateSeccompProfile(packages []string, ports []int) (*SeccompProfile, error) {
	// Base set of allowed syscalls for any minimal container
	baseSyscalls := []string{
		"accept", "accept4", "access", "arch_prctl", "bind", "brk",
		"capget", "capset", "chdir", "chown", "chown32", "clock_getres",
		"clock_gettime", "clock_nanosleep", "clone", "close", "connect",
		"dup", "dup2", "dup3", "epoll_create", "epoll_create1",
		"epoll_ctl", "epoll_pwait", "epoll_wait", "eventfd", "eventfd2",
		"execve", "exit", "exit_group", "faccessat", "faccessat2",
		"fadvise64", "fallocate", "fchdir", "fchmod", "fchmodat",
		"fchown", "fchown32", "fchownat", "fcntl", "fdatasync",
		"flock", "fork", "fstat", "fstatfs", "fsync", "ftruncate",
		"futex", "getcwd", "getdents", "getdents64", "getegid",
		"geteuid", "getgid", "getgroups", "getpeername", "getpgrp",
		"getpid", "getppid", "getpriority", "getrandom", "getrlimit",
		"getrusage", "getsockname", "getsockopt", "gettid",
		"gettimeofday", "getuid", "ioctl", "kill", "lchown",
		"listen", "lseek", "lstat", "madvise", "memfd_create",
		"mincore", "mkdir", "mkdirat", "mmap", "mount", "mprotect",
		"mremap", "msgctl", "msgget", "msgrcv", "msgsnd", "msync",
		"munmap", "nanosleep", "newfstatat", "open", "openat",
		"pause", "pipe", "pipe2", "poll", "ppoll", "prctl",
		"pread64", "preadv", "prlimit64", "pselect6", "pwrite64",
		"pwritev", "read", "readlink", "readlinkat", "readv",
		"recvfrom", "recvmmsg", "recvmsg", "rename", "renameat",
		"renameat2", "restart_syscall", "rmdir", "rt_sigaction",
		"rt_sigpending", "rt_sigprocmask", "rt_sigqueueinfo",
		"rt_sigreturn", "rt_sigsuspend", "rt_sigtimedwait",
		"sched_getaffinity", "sched_yield", "seccomp", "select",
		"semctl", "semget", "semop", "sendfile", "sendmmsg",
		"sendmsg", "sendto", "set_robust_list", "set_tid_address",
		"setgid", "setgroups", "setitimer", "setpgid", "setsid",
		"setsockopt", "setuid", "shutdown", "sigaltstack", "socket",
		"socketpair", "splice", "stat", "statfs", "statx",
		"symlink", "symlinkat", "sync", "sysinfo", "tee", "tgkill",
		"timer_create", "timer_delete", "timer_getoverrun",
		"timer_gettime", "timer_settime", "timerfd_create",
		"timerfd_gettime", "timerfd_settime", "times", "tkill",
		"truncate", "umask", "uname", "unlink", "unlinkat",
		"utimensat", "vfork", "wait4", "waitid", "write", "writev",
	}

	// If network ports are requested, add network-specific syscalls
	if len(ports) > 0 {
		baseSyscalls = append(baseSyscalls, "listen", "bind", "accept", "accept4")
	}

	profile := &SeccompProfile{
		DefaultAction: "SCMP_ACT_ERRNO",
		Architectures: []string{"SCMP_ARCH_X86_64", "SCMP_ARCH_AARCH64"},
		Syscalls: []SyscallRule{
			{
				Names:  baseSyscalls,
				Action: "SCMP_ACT_ALLOW",
			},
		},
	}

	return profile, nil
}

// GenerateLandlockPolicy constructs a Landlock policy from the declared filesystem layout.
func GenerateLandlockPolicy(filePaths []string, readOnly bool) (*LandlockPolicy, error) {
	policy := &LandlockPolicy{
		ReadablePaths:   []string{"/usr", "/lib", "/etc/ssl", "/etc/passwd", "/etc/group"},
		WritablePaths:   []string{"/tmp", "/var/log"},
		ExecutablePaths: []string{"/usr/bin", "/usr/sbin"},
	}

	for _, fp := range filePaths {
		policy.ReadablePaths = append(policy.ReadablePaths, fp)
		if !readOnly {
			policy.WritablePaths = append(policy.WritablePaths, fp)
		}
	}

	return policy, nil
}

// MarshalSeccomp serializes a Seccomp profile to indented JSON.
func MarshalSeccomp(profile *SeccompProfile) ([]byte, error) {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal seccomp profile: %w", err)
	}
	return data, nil
}

// MarshalLandlock serializes a Landlock policy to indented JSON.
func MarshalLandlock(policy *LandlockPolicy) ([]byte, error) {
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal landlock policy: %w", err)
	}
	return data, nil
}
