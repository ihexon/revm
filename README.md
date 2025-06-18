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

You can given a virtual disk into vm, but you need to format the disk and mount into somewhere:
```shell
./revm --rootfs ~/alpine_rootfs  --data-disk ~/disk --  /bin/sh

# a more complex example
./out/bin/revm-arm64 \
  --envs "HOME=/root"  \
  --rootfs ~/ubuntu \
  --memory 1024 \
  --mount /Users:/Users  \
  --mount /tmp:/mytmp  \
  --data-disk ~/data.img \
  --data-disk ~/data1.img  -- /bin/bash

vm $ mkfs.ext4 /dev/vda && mkfs.ext4 /dev/vdb
vm $ mount /dev/vda /mnt/vda && mount /dev/vdb /mnt/vdb
```

## Help message

```go
NAME:
   ./out/bin/revm-arm64 - run a linux shell in 1 second

USAGE:
   ./out/bin/revm-arm64 [command] [flags]

DESCRIPTION:
   run a linux shell in 1 second

GLOBAL OPTIONS:
   --rootfs string                            rootfs path, e.g. /var/lib/libkrun/rootfs/alpine-3.15.0
   --cpus int                                 given how many cpu cores (default: 1)
   --memory int                               set memory in MB (default: 512)
   --envs string [ --envs string ]            set envs for cmdline, e.g. --envs=FOO=bar --envs=BAZ=qux
   --data-disk string [ --data-disk string ]  set data disk path, the disk will be map into /dev/vdX
   --mount string [ --mount string ]          mount host dir to guest dir
   --help, -h                                 show help
```



