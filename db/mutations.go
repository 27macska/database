package db

import (
	"fmt"
	"strconv"
)

// The doXXX functions below are the pure index-mutation core for each
// write command: they validate arguments and apply the change to d.idx,
// nothing else. They are called both from Execute (which additionally
// logs the mutation and formats a reply) and from applyReplayed (which
// only needs the mutation, since the command was already logged once).

func (d *DB) doSET(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	d.idx.SetString(args[0], args[1])
	return nil
}

func (d *DB) doDEL(args []string) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("wrong number of arguments")
	}
	count := 0
	for _, key := range args {
		if d.idx.Delete(key) {
			count++
		}
	}
	return count, nil
}

func (d *DB) doMSET(args []string) error {
	if len(args) == 0 || len(args)%2 != 0 {
		return fmt.Errorf("wrong number of arguments, expected key/value pairs")
	}
	for i := 0; i < len(args); i += 2 {
		d.idx.SetString(args[i], args[i+1])
	}
	return nil
}

// doEXPIREAT applies an absolute unix-second expiry. This is the logged
// form of the user-facing EXPIRE command (which takes a relative number of
// seconds); Execute converts relative-to-absolute before logging so replay
// always reconstructs the same wall-clock expiry regardless of when the
// log is replayed.
func (d *DB) doEXPIREAT(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	at, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid expiry timestamp %q", args[1])
	}
	d.idx.SetExpireAt(args[0], at, d.Clock())
	return nil
}

func (d *DB) doHSET(args []string) error {
	if len(args) < 3 || (len(args)-1)%2 != 0 {
		return fmt.Errorf("wrong number of arguments, expected key + field/value pairs")
	}
	key := args[0]
	now := d.Clock()
	for i := 1; i < len(args); i += 2 {
		if err := d.idx.HSet(key, args[i], args[i+1], now); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) doLPUSH(args []string) (int, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("wrong number of arguments")
	}
	return d.idx.LPush(args[0], args[1:], d.Clock())
}

func (d *DB) doRPUSH(args []string) (int, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("wrong number of arguments")
	}
	return d.idx.RPush(args[0], args[1:], d.Clock())
}

func (d *DB) doINCR(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("wrong number of arguments")
	}
	return d.idx.Incr(args[0], 1, d.Clock())
}

func (d *DB) doDECR(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("wrong number of arguments")
	}
	return d.idx.Incr(args[0], -1, d.Clock())
}

func (d *DB) doFLUSHDB(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("wrong number of arguments")
	}
	d.idx.Clear()
	return nil
}
