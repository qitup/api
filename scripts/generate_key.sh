#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
KEY_PATH="$DIR/../keys/sign-$ENV"

function write_key {
    key_path=$1

    openssl rand -base64 32 > ${key_path}
    chmod 0600 ${key_path}
}


if [ ! -f ${KEY_PATH} ]; then
    write_key $KEY_PATH
fi