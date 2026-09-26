package main

import (
	"fmt"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("vim-go")
	_ = godotenv.Load() // 自动读取当前目录下的 .env 文件

}
