//go:build !darwin && !linux

package localruntime

import (
	"context"
	"errors"
)

func Supported() bool { return false }
func RunNative(context.Context, string, string, string, string, string, string) error {
	return errors.New("native local sessions require macOS or Linux")
}
func (*Client) Ensure(context.Context) error { return errors.New("local PTYs require macOS or Linux") }
func Run(context.Context, string) error      { return errors.New("local PTYs require macOS or Linux") }
