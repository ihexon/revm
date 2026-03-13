pullcode_rpi() {
    rsync -avz --delete  --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p 8522" ihexon@192.168.1.251:/home/ihexon/revm/ "$(pwd)/"
}

pushcode_rpi() {
    rsync -avz --delete  --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p 8522" "$(pwd)/" ihexon@192.168.1.251:/home/ihexon/revm/
}

pullcode_rk5b() {
    rsync -avz --delete  --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p 22" ihexon@192.168.1.252:/home/ihexon/revm/ "$(pwd)/"
}

pushcode_rk5b() {
    rsync -avz --delete  --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p 22" "$(pwd)/" ihexon@192.168.1.252:/home/ihexon/revm/
}
