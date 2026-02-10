# Guest Agent

The guest agent runs inside the Linux VM as a child of PID 1. It bootstraps the guest environment — mounting filesystems, configuring networking, starting services (SSH, Podman API), and executing user commands.
