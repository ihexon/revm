# revm is the re-evolution of linux vm

## download any rootfs
```shell
$ wget https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/aarch64/alpine-minirootfs-3.21.3-aarch64.tar.gz
$ mkdir ~/alpine_rootfs && tar -xvf -C ~/alpine_rootfs
```

## run the rootfs


```shell
./revm --rootfs ~/alpine_rootfs --  /bin/sh
```

You need to run `/dhclient4-linux-arm64` get ip address from network stack(provided by gvproxy) when get into vm shell:
```shell
vm shell $ /dhclient4-linux-arm64
vm shell $ ifconfig # or ip addr
```
`dhclient4-linux-arm64` is a custom self-contained dhclient4 used to configure the network interface. 


You can given a virtual disk into vm, but you need to format the disk and mount into somewhere:
```shell
./revm --rootfs ~/alpine_rootfs  --data-disk ~/disk --  /bin/sh

vm $ mkfs.ext4 /dev/vda
vm $ mount /dev/vda /mnt
```




## Help message

```
NAME:
./bin/main - run a linux shell in 1 second

USAGE:
./bin/main [command] [flags]

DESCRIPTION:
run a linux shell in 1 second

GLOBAL OPTIONS:
--rootfs string                  rootfs path, e.g. /var/lib/libkrun/rootfs/alpine-3.15.0
--cpus int                       given how many cpu cores (default: 1)
--memory int                     set memory in MB (default: 512)
--envs string [ --envs string ]  set envs for cmdline, e.g. --envs=FOO=bar --envs=BAZ=qux
--data-disk string               set data disk path, the disk will be map into /dev/vdX
--help, -h                       show help
```



