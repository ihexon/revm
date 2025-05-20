package main

import (
	"github.com/sirupsen/logrus"
	"linuxvm/pkg/startup"
)

const (
	eth0     = "eth0"
	attempts = 1
	verbose  = true
)

func main() {
	err := startup.DHClient4(eth0, attempts, verbose)
	if err != nil {
		logrus.Fatal(err.Error())
	}
}
