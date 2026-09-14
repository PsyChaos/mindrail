package migration

import (
	"regexp"
	"strings"
)

// This file reads the column names out of a CREATE TABLE and out of an
// ALTER TABLE ... ADD COLUMN, and nothing else.
//
// It is the narrowest thing that answers finding F9: a table that was dropped
// and recreated by hand with a different shape satisfies "the object exists",
// so `doctor` reported the schema healthy, `init` then failed on a column that
// was not there, and the remedy it printed sent the user back to `doctor`.
// Comparing names is enough to break that loop and is far short of a schema
// diff -- types, constraints, defaults and collations are not compared, and a
// table any recorded migration has ALTERed in a form other than ADD COLUMN is
// not compared at all.
//
// ADD COLUMN is the one form the ledger reads exactly, since MR-004 (decision
// D-73): it adds a name and moves nothing, so the expected list is the CREATE's
// names plus the added one. Without it, the first migration to add a column —
// 000003's `revision` on `tasks` — would have silently removed that table from
// the F9 check. Every other form (DROP COLUMN, RENAME COLUMN, RENAME TO) still
// makes the checker forget the table rather than model it.
//
// All of it runs at Load time against embedded files, so the warm path pays for
// one extra query and no parsing.

// createTablePattern locates the head of a CREATE TABLE so the body after it
// can be read. It intentionally repeats effectPattern's shape rather than
// reusing it: this one needs match offsets, and merging the two would make a
// pattern that serves neither clearly.
var createTablePattern = regexp.MustCompile(
	`(?im)^[\t ]*CREATE[\t ]+((?:(?:UNIQUE|TEMP|TEMPORARY|VIRTUAL)[\t ]+)*)TABLE[\t ]+(?:IF[\t ]+NOT[\t ]+EXISTS[\t ]+)?(` + identifier + `)`)

// alterTablePattern matches every form of ALTER TABLE, and reads the added
// column's name when the form is ADD [COLUMN]. The second group is empty for
// every other form, and an empty second group is the reason to stop trusting
// the recorded column list.
var alterTablePattern = regexp.MustCompile(
	`(?im)^[\t ]*ALTER[\t ]+TABLE[\t ]+(` + identifier + `)` +
		`(?:[\t ]+ADD[\t ]+(?:COLUMN[\t ]+)?(` + identifier + `))?`)

// tableConstraintHeads are the words a table constraint can start with. An item
// in the column list that starts with one of them is not a column, and treating
// it as one would invent a column no database has and fail every healthy
// schema.
var tableConstraintHeads = map[string]struct{}{
	"CONSTRAINT": {},
	"PRIMARY":    {},
	"FOREIGN":    {},
	"UNIQUE":     {},
	"CHECK":      {},
}

// tableColumns maps each table this body creates to the column names it
// declares, lower-cased because SQLite compares identifiers case-insensitively.
//
// A CREATE TABLE with no parenthesised column list -- `CREATE TABLE x AS
// SELECT ...` -- is skipped rather than guessed at, and a table that is skipped
// is simply not shape-checked.
func tableColumns(body string) map[string][]string {
	columns := make(map[string][]string)

	for _, loc := range createTablePattern.FindAllStringSubmatchIndex(body, -1) {
		if strings.Contains(strings.ToUpper(body[loc[2]:loc[3]]), "TEMP") {
			continue
		}
		names, ok := columnList(body[loc[1]:])
		if !ok || len(names) == 0 {
			continue
		}
		columns[unquote(body[loc[4]:loc[5]])] = names
	}

	if len(columns) == 0 {
		return nil
	}
	return columns
}

// alterations reads every ALTER TABLE in body and sorts it into the one form
// the ledger can follow and the forms it cannot: added maps a table to the
// columns ADD COLUMN gave it, in file order and lower-cased like tableColumns;
// forgotten names every table some other form touched, once each.
//
// A table can appear in both — an ADD COLUMN and a DROP COLUMN in one file —
// and the caller forgets it, because the drop is the half it cannot follow.
func alterations(body string) (added map[string][]string, forgotten []string) {
	matches := alterTablePattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		table := unquote(match[1])
		if column := match[2]; column != "" {
			if added == nil {
				added = make(map[string][]string)
			}
			added[table] = append(added[table], strings.ToLower(unquote(column)))
			continue
		}
		if _, duplicate := seen[table]; duplicate {
			continue
		}
		seen[table] = struct{}{}
		forgotten = append(forgotten, table)
	}
	return added, forgotten
}

// columnList reads the parenthesised column list that starts in rest and
// returns the column names in it. The bool is false when there is no such list
// or it is unbalanced, which is the signal to shape-check nothing.
//
// The scan is depth- and quote-aware because the separators it looks for occur
// inside things that are not separators: `CHECK (a > 0 AND b < 1)` nests
// parentheses, `DEFAULT 'a,b'` hides a comma in a string, and a trailing `--`
// note can hide anything at all.
func columnList(rest string) ([]string, bool) {
	rest = stripComments(rest)

	open := strings.IndexRune(rest, '(')
	if open < 0 {
		return nil, false
	}
	if end := strings.IndexRune(rest, ';'); end >= 0 && end < open {
		return nil, false
	}

	var (
		names []string
		item  strings.Builder
		depth = 1
		quote rune
	)

	for _, r := range rest[open+1:] {
		if quote != 0 {
			item.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}

		switch r {
		case '\'', '"', '`':
			quote = r
			item.WriteRune(r)
		case '[':
			quote = ']'
			item.WriteRune(r)
		case '(':
			depth++
			item.WriteRune(r)
		case ')':
			depth--
			if depth == 0 {
				if name, ok := columnName(item.String()); ok {
					names = append(names, name)
				}
				return names, true
			}
			item.WriteRune(r)
		case ',':
			if depth > 1 {
				item.WriteRune(r)
				continue
			}
			if name, ok := columnName(item.String()); ok {
				names = append(names, name)
			}
			item.Reset()
		default:
			item.WriteRune(r)
		}
	}

	return nil, false
}

// columnName reads the name off one item of a column list. The bool is false
// for a table constraint and for an empty item, neither of which names a
// column.
func columnName(item string) (string, bool) {
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return "", false
	}

	name, _ := firstToken(trimmed)
	if name == "" {
		return "", false
	}
	if _, isConstraint := tableConstraintHeads[strings.ToUpper(name)]; isConstraint {
		return "", false
	}
	return strings.ToLower(name), true
}

// firstToken splits off the leading identifier, honouring the four ways SQLite
// lets one be quoted. The remainder is returned so a caller can keep reading;
// nothing in this file needs it yet, and returning it costs nothing.
func firstToken(s string) (string, string) {
	closing := map[rune]rune{'"': '"', '`': '`', '[': ']', '\'': '\''}

	runes := []rune(s)
	if end, quoted := closing[runes[0]]; quoted {
		for i := 1; i < len(runes); i++ {
			if runes[i] == end {
				return string(runes[1:i]), string(runes[i+1:])
			}
		}
		return "", ""
	}

	for i, r := range runes {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' {
			return string(runes[:i]), string(runes[i:])
		}
	}
	return s, ""
}

// stripComments removes `--` and `/* */` comments that are not inside a string
// literal, replacing them with a space so the tokens on either side stay apart.
func stripComments(s string) string {
	var (
		out   strings.Builder
		runes = []rune(s)
		quote rune
	)
	out.Grow(len(s))

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if quote != 0 {
			out.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}

		switch {
		case r == '\'' || r == '"' || r == '`':
			quote = r
			out.WriteRune(r)
		case r == '[':
			quote = ']'
			out.WriteRune(r)
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			out.WriteRune('\n')
		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i++
			out.WriteRune(' ')
		default:
			out.WriteRune(r)
		}
	}

	return out.String()
}
