package update

import (
	"bufio"
	"io"
	"strings"
)

// Confirm 从 r 读取一行，Y/y/回车 → true（安装），N/n → false（取消），
// 其他输入重新询问；EOF（非交互管道）→ false。
func Confirm(r io.Reader) (bool, error) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}
