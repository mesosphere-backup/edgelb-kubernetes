#!/bin/sh
# NOTE: this script should be sourced, not executed to ensure that the ssh
# configuration is available to the environment.
[ ! -z $ssh_git_key ] || ssh_git_key=$SSH_GIT_KEY
eval $(ssh-agent)
ssh-add $SSHDIR/$ssh_git_key
ssh -T git@github.com
