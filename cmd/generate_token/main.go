package main

import (
	"fmt"

	pkgjwt "github.com/damoang/angple-backend/pkg/jwt"
)

func main() {
	secret := "local-dev-secret-key"

	// JWT Manager 생성
	jwtManager := pkgjwt.NewManager(secret, 900, 604800)

	// Access Token 생성 (관리자 레벨 10)
	token, err := jwtManager.GenerateAccessToken("admin", "관리자", 10)
	if err != nil {
		panic(err)
	}

	fmt.Println(token)
}
