//go:build !darwin && !linux

package localruntime

import (
	"context"
	"errors"
)

func Supported() bool                        { return false }
func (*Client) Ensure(context.Context) error { return errors.New("local PTYs require macOS or Linux") }
func Run(context.Context, string) error      { return errors.New("local PTYs require macOS or Linux") }
