#! /usr/bin/env sh

# this script is effectively a compatibility layer
# due to https://github.com/oomol/oomol-studio-code/blob/96b3a492f29f581319cfe13c21d5dce400a120ee/oomol-studio-main/desktop/container-server/image/sh/load_images.sh#L17.
#
# this script is solely responsible for terminating the Podman API service; the guest-agent will automatically restart it.
#
# Usage: ./podman stop
#        ./podman start # do nothing

main() {
    killall podman
}

main "$@"
