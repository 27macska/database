package db

import (
	"fmt"
	"strconv"
	"strings"

	"kvstore/index"
)

// Execute runs one already-tokenized command (tokens[0] is the command
// name, the rest are its arguments) and returns the text to print for it.
//
// The returned error is reserved for two things: ErrExit (the client sent
// EXIT) and fatal durability failures (the log file could not be written
// to disk). Every ordinary command mistake -- wrong argument count,
// unknown command, wrong value type, non-integer input -- is reported as
// part of the returned string (prefixed "ERR ") with a nil error, so
// callers can't accidentally treat "bad user input" as "the store is
// broken".
func (d *DB) Execute(tokens []string) (string, error) {
	if len(tokens) == 0 {
		return "", nil
	}
	cmd := strings.ToUpper(tokens[0])
	args := tokens[1:]

	switch cmd {
	case "SET":
		if err := d.doSET(args); err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("SET", args); err != nil {
			return "", err
		}
		return "OK", nil

	case "GET":
		if len(args) != 1 {
			return errArgs("GET"), nil
		}
		e, ok := d.idx.Get(args[0], d.Clock())
		if !ok {
			return nilStr, nil
		}
		if e.Type != index.TypeString {
			return errWrongType(), nil
		}
		return e.Str, nil

	case "DEL":
		n, err := d.doDEL(args)
		if err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("DEL", args); err != nil {
			return "", err
		}
		return strconv.Itoa(n), nil

	case "EXISTS":
		if len(args) == 0 {
			return errArgs("EXISTS"), nil
		}
		now := d.Clock()
		count := 0
		for _, key := range args {
			if d.idx.Exists(key, now) {
				count++
			}
		}
		return strconv.Itoa(count), nil

	case "MSET":
		if err := d.doMSET(args); err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("MSET", args); err != nil {
			return "", err
		}
		return "OK", nil

	case "MGET":
		if len(args) == 0 {
			return errArgs("MGET"), nil
		}
		now := d.Clock()
		lines := make([]string, len(args))
		for i, key := range args {
			e, ok := d.idx.Get(key, now)
			if !ok || e.Type != index.TypeString {
				lines[i] = nilStr
			} else {
				lines[i] = e.Str
			}
		}
		return strings.Join(lines, "\n"), nil

	case "EXPIRE":
		if len(args) != 2 {
			return errArgs("EXPIRE"), nil
		}
		seconds, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return "ERR value is not an integer or out of range", nil
		}
		now := d.Clock()
		at := now + seconds
		if !d.idx.SetExpireAt(args[0], at, now) {
			return "0", nil
		}
		if err := d.logMutation("EXPIREAT", []string{args[0], strconv.FormatInt(at, 10)}); err != nil {
			return "", err
		}
		return "1", nil

	case "TTL":
		if len(args) != 1 {
			return errArgs("TTL"), nil
		}
		ttl, ok := d.idx.TTL(args[0], d.Clock())
		if !ok {
			return "-2", nil
		}
		return strconv.FormatInt(ttl, 10), nil

	case "RANGE":
		if len(args) != 2 {
			return errArgs("RANGE"), nil
		}
		entries := d.idx.Range(args[0], args[1], d.Clock())
		if len(entries) == 0 {
			return emptyStr, nil
		}
		lines := make([]string, len(entries))
		for i, e := range entries {
			lines[i] = e.Key + " " + e.Value
		}
		return strings.Join(lines, "\n"), nil

	case "BEGIN":
		if len(args) != 0 {
			return errArgs("BEGIN"), nil
		}
		if d.inTx {
			return "ERR transaction already in progress", nil
		}
		d.snapshot = d.idx.Clone()
		d.pending = nil
		d.inTx = true
		return "OK", nil

	case "COMMIT":
		if len(args) != 0 {
			return errArgs("COMMIT"), nil
		}
		if !d.inTx {
			return "ERR no transaction in progress", nil
		}
		if err := d.log.AppendBatch(d.pending); err != nil {
			return "", err
		}
		d.inTx = false
		d.snapshot = nil
		d.pending = nil
		return "OK", nil

	case "ABORT":
		if len(args) != 0 {
			return errArgs("ABORT"), nil
		}
		if !d.inTx {
			return "ERR no transaction in progress", nil
		}
		d.idx.Restore(d.snapshot)
		d.inTx = false
		d.snapshot = nil
		d.pending = nil
		return "OK", nil

	case "HSET":
		if err := d.doHSET(args); err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("HSET", args); err != nil {
			return "", err
		}
		return "OK", nil

	case "HGET":
		if len(args) != 2 {
			return errArgs("HGET"), nil
		}
		v, ok, err := d.idx.HGet(args[0], args[1], d.Clock())
		if err != nil {
			return errWrongType(), nil
		}
		if !ok {
			return nilStr, nil
		}
		return v, nil

	case "HGETALL":
		if len(args) != 1 {
			return errArgs("HGETALL"), nil
		}
		fields, err := d.idx.HGetAll(args[0], d.Clock())
		if err != nil {
			return errWrongType(), nil
		}
		if len(fields) == 0 {
			return emptyStr, nil
		}
		lines := make([]string, len(fields))
		for i, f := range fields {
			lines[i] = f.Field + " " + f.Value
		}
		return strings.Join(lines, "\n"), nil

	case "LPUSH":
		n, err := d.doLPUSH(args)
		if err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("LPUSH", args); err != nil {
			return "", err
		}
		return strconv.Itoa(n), nil

	case "RPUSH":
		n, err := d.doRPUSH(args)
		if err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("RPUSH", args); err != nil {
			return "", err
		}
		return strconv.Itoa(n), nil

	case "LRANGE":
		if len(args) != 3 {
			return errArgs("LRANGE"), nil
		}
		start, err1 := strconv.Atoi(args[1])
		stop, err2 := strconv.Atoi(args[2])
		if err1 != nil || err2 != nil {
			return "ERR start/stop must be integers", nil
		}
		values, err := d.idx.LRange(args[0], start, stop, d.Clock())
		if err != nil {
			return errWrongType(), nil
		}
		if len(values) == 0 {
			return emptyStr, nil
		}
		return strings.Join(values, "\n"), nil

	case "INCR":
		n, err := d.doINCR(args)
		if err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("INCR", args); err != nil {
			return "", err
		}
		return strconv.FormatInt(n, 10), nil

	case "DECR":
		n, err := d.doDECR(args)
		if err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("DECR", args); err != nil {
			return "", err
		}
		return strconv.FormatInt(n, 10), nil

	case "FLUSHDB":
		if err := d.doFLUSHDB(args); err != nil {
			return errStr(err), nil
		}
		if err := d.logMutation("FLUSHDB", args); err != nil {
			return "", err
		}
		return "OK", nil

	case "EXIT":
		return "", ErrExit

	default:
		return fmt.Sprintf("ERR unknown command %q", tokens[0]), nil
	}
}

func errArgs(cmd string) string {
	return fmt.Sprintf("ERR wrong number of arguments for '%s' command", strings.ToLower(cmd))
}

func errWrongType() string {
	return "ERR " + index.ErrWrongType.Error()
}

func errStr(err error) string {
	return "ERR " + err.Error()
}
