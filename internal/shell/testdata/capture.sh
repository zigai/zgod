#!/bin/sh
exec "$ZGOD_TEST_BINARY" -test.run=TestShellBoundaryProcess -- "$@"
