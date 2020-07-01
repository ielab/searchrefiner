#!/usr/bin/env bash

currentDir=$(pwd)
for plugin in $(ls plugin)
do
  plugPath=plugin/${plugin}
  cd ${plugPath}
  echo "updating ${plugin}..."
  git pull
  cd ${currentDir}
done