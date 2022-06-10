echo "Installing correct binaries for os..."
ARCH=$(uname -m)
OS=$(uname -s)
if [ "$ARCH" = "x86_64" ]; then
  ARCH="amd64"
fi
if [ "$ARCH" = "aarch64" ]; then
  ARCH="arm64"
fi
ln -sf "bin/$OS/$ARCH/fermenter" "$PWD/fermenter"