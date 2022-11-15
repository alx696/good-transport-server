#!/bin/sh
set -e

go mod tidy
go mod download

mkdir -p build

echo "开始构建Linux"
go build -o "build/gts-amd64"
echo "完成构建Linux"

echo "开始构建Windows"
GOOS=windows go build -o "build/gts-amd64.exe"
echo "完成构建Windows"

echo "开始构建Android ARM64"
GOOS=android GOARCH=arm64 go build -o "build/gts-android_arm64"
echo "完成构建Android ARM64"

echo "开始构建Android AAR"
echo "NDK路径: ${ANDROID_NDK_HOME}"
if [ -z "$(which gomobile)" ]; then
  echo "没有安装gomobile, 安装gomobile"
  go install golang.org/x/mobile/cmd/gomobile@latest
  gomobile init
else
  echo "已经安装gomobile"
fi
go get -d golang.org/x/mobile/cmd/gomobile
gomobile bind -target=android/arm64,android/arm,android/amd64 -o "build/gts-android.aar" github.com/alx696/go-less/lilu_net ./http_server ./qc
rm "build/gts-android-sources.jar"
echo "完成构建Android AAR"

go mod tidy

echo "复制模板"
tar zcf template.tar.gz template
mv template.tar.gz build/
