echo "Build Script v1"
echo "Building for amd64..."
GOARCH=amd64 GOOS=linux go build -o bin/linux/amd64/fermenter main.go
GOARCH=amd64 GOOS=darwin go build -o bin/darwin/amd64/fermenter main.go
echo "Building for arm64..."
GOARCH=arm64 GOOS=linux go build -o bin/linux/arm64/fermenter main.go
GOARCH=arm64 GOOS=darwin go build -o bin/darwin/arm64/fermenter main.go
echo "Done"