//go:build windows

package cli

import "os"

func openTTY() (input *os.File, output *os.File, cleanup func(), err error) {
	input, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}

	output, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		_ = input.Close()
		return nil, nil, nil, err
	}

	cleanup = func() {
		_ = output.Close()
		_ = input.Close()
	}

	return input, output, cleanup, nil
}
