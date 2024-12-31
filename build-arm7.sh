#!/bin/bash

# Set the target architecture and OS
export GOARCH=arm
export GOARM=7
export GOOS=linux

# Set the output binary name using the organization name
OUTPUT_BINARY="status-updater-arm7"

# Extract the version number from the control file
VERSION=$(grep '^Version:' status-updater/DEBIAN/control | awk '{print $2}')

# Build the Go application
echo "Building for $GOOS/$GOARCH (ARMv7)..."
go build -o $OUTPUT_BINARY main.go

cp $OUTPUT_BINARY /opt/status-updater/status-updater/opt/status-updater/status-updater
cp $OUTPUT_BINARY /opt/status-updater/status-updater-buildroot/opt/status-updater/status-updater

# Build the .deb package with the version number in the name
dpkg-deb --build -Zgzip status-updater "status-updater_${VERSION}.deb"
#Unset the target architecture and OS
unset GOARCH
unset GOARM
unset GOOS

echo "Building tarball status-updater_$VERSION.tar.xz"
tar -cJf status-updater_$VERSION.tar.xz status-updater-buildroot/

# Check if the build was successful
if [ $? -eq 0 ]; then
    echo "Build successful! Output binary: $OUTPUT_BINARY"
else
    echo "Build failed!"
    exit 1
fi