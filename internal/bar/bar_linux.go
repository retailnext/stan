// Copyright (c) 2018, RetailNext, Inc.
// All rights reserved.

package bar

import "syscall"

var sharedVar string

const sharedConst = 456

func shared() {
}

func linuxSpecific() {
	var sysInfo syscall.Sysinfo_t
	syscall.Sysinfo(&sysInfo)
}
