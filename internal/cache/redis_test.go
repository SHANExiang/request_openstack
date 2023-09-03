package cache

import (
	"runtime"
	"strings"
	"testing"
)



func TestNewClusterClient(t *testing.T) {
	methodName := GetCurrentMethodName()

	// 输出当前方法名称
	println(methodName)
}