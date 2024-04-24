#!/bin/bash
image=$1

if [[ $image == "" ]]
then
    echo "usage: $0 <image_name>"
    exit
fi
echo "building $image"
sudo docker build -t $image .
sudo docker push $image
