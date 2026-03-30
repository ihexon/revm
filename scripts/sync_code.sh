export SSH_PORT=10024
export LOCAL_SRC_DIR="$(pwd)/"
export SSH_REMOTE_ADDR="ihexon@192.168.1.210:/home/ihexon/revm/"

pullcode ()
{
    rsync -avz --delete --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p $SSH_PORT" "$SSH_REMOTE_ADDR" "$LOCAL_SRC_DIR"
}

pushcode ()
{
    rsync -avz --delete --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p $SSH_PORT" "$LOCAL_SRC_DIR" "$SSH_REMOTE_ADDR"
}