#!/bin/bash -xe
# test-setup.sh - Install required stuffs
# Used in both CI jobs and locally
#
# Install the following tools:
# * godep

# Get OS
case $(uname -s) in
    Darwin)
        OS=darwin
        ;;
    Linux)
        if LSB_RELEASE=$(which lsb_release); then
            OS=$($LSB_RELEASE -s -c)
        else
            # No lsb-release, trya hack or two
            if which dpkg 1>/dev/null; then
                OS=debian
            elif which yum 1>/dev/null || which dnf 1>/dev/null; then
                OS=redhat
            else
                echo "Linux distro not yet supported"
                exit 1
            fi
        fi
        ;;
    *)
        echo "Unsupported OS"
        exit 1
        ;;
esac

if which go 1>/dev/null; then
    if ! which go 1>/dev/null; then
        go get -u -v github.com/tools/godep
    fi
else
    echo "go not found, install golang from source?"
fi
