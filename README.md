# revm helps you quickly run a lightweight Linux command-line environment

You don’t need a bootable Linux image, nor do you need to install an entire distribution from a Linux ISO.  
All you need is a Linux rootfs to quickly launch a lightweight Linux command-line environment.

The rootfs can be obtained from a `docker export`, and you can even directly launch a statically compiled Linux ELF executable.  
This makes revm interesting to play with in many scenarios.

## Run a Linux shell in 1 second
```shell
# download alpine rootfs
$ wget https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/aarch64/alpine-minirootfs-3.21.3-aarch64.tar.gz
$ mkdir ~/alpine_rootfs && tar -xvf -C ~/alpine_rootfs

# run a shell
./revm --rootfs ~/alpine_rootfs --  /bin/sh

```


## Subcommand help message
### run 
```shell
NAME:
   ./out/bin/revm run - run the rootfs

USAGE:
   ./out/bin/revm run [command [command options]]

OPTIONS:
   --rootfs string                            rootfs path, e.g. /var/lib/libkrun/rootfs/alpine-3.15.0
   --cpus int                                 given how many cpu cores (default: 8)
   --memory int                               set memory in MB (default: 24576)
   --envs string [ --envs string ]            set envs for cmdline, e.g. --envs=FOO=bar --envs=BAZ=qux
   --data-disk string [ --data-disk string ]  set data disk path, the disk will be map into /dev/vdX
   --mount string [ --mount string ]          mount host dir to guest dir
   --system-proxy                             use system proxy, set environment http(s)_proxy to guest (default: false)
   --help, -h                                 show help
```

### attach
```shell
NAME:
   ./out/bin/revm attach - attach to the console of the running guest

USAGE:
   attach [rootfs]

OPTIONS:
   --help, -h  show help
```

# Binary signing issues on MacOS

MacOS does not allow running external binaries, so you have to build re-vm from source code or sign the binary manually. 

For build the binary from the source code. you need to have golang development environment (using [brew](https://brew.sh)), and run the build script:
```shell
./build.sh # run build script
```

Or download binaries from release, and remove `com.apple.quarantine` for binaries.
```shell
xattr -d com.apple.quarantine ./bin/revm-arm64
xattr -d com.apple.quarantine ./lib/*
```

# Bug report

For bug reports and feature suggestions, please open an [issue](https://github.com/ihexon/revm/issues).