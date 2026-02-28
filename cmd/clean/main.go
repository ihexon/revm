// Standalone clean binary — polls PPID and removes the workspace directory
// once the parent process exits. Built with CGO_ENABLED=0 for static linking.
// Zero external dependencies — stdlib only.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	workspace := flag.String("workspace", "", "workspace directory to remove after parent exits")
	flag.Parse()

	if *workspace == "" {
		return
	}

	absWs, err := filepath.Abs(filepath.Clean(*workspace))
	if err != nil {
		log.Fatal(err)
	}

	for {
		if os.Getppid() == 1 {
			log.Printf("PPID=1, removing workspace %q", absWs)
			if _, err := os.Stat(absWs); err == nil {
				if err := os.RemoveAll(absWs); err != nil {
					log.Fatal(err)
				}
			}
			return
		}
		time.Sleep(1 * time.Second)
	}
}
