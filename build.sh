#!/bin/sh
set -e

go mod tidy
go mod download

mkdir -p build

echo "开始构建Linux 64位"
go build -o "build/gts-linux-64"
echo "完成构建Linux 64位"

echo "开始构建Linux 32位"
GOARCH=386 go build -o "build/gts-linux-32"
echo "完成构建Linux 32位"

echo "开始构建Windows 64位"
GOOS=windows go build -o "build/gts-windows-64.exe"
echo "完成构建Windows 64位"

echo "开始构建Android ARM64"
GOOS=android GOARCH=arm64 go build -o "build/gts-android_arm64"
echo "完成构建Android ARM64"

echo "开始构建Android ARM32"
GOOS=linux GOARCH=arm go build -o "build/gts-android_arm32"
echo "完成构建Android ARM32"

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
