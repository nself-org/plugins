//! Transaction-control detection for migration files (contract plugin.boot-migrations v1).
//!
//! Mirrors cli sdk/go/migrate/txcontrol.go. The runner wraps every file in its own transaction, so
//! a file that ends it (COMMIT) would leave its effects committed with no ledger row. This is a
//! lexer, not a parser: it skips comments, '...' and E'...' strings, "..." identifiers and
//! $tag$...$tag$ bodies, then looks at the first word(s) of each top-level statement.

#[derive(Debug, Clone, PartialEq)]
struct Token {
    word: String, // upper-cased word, or ";" for a statement end
    line: usize,
}

const TX_WORDS: &[&str] = &[
    "BEGIN", "COMMIT", "ROLLBACK", "END", "ABORT", "SAVEPOINT", "RELEASE",
];

/// Returns the first transaction-control statement in `sql` and its line.
pub fn find_tx_control(sql: &str) -> Option<(String, usize)> {
    let mut stmt: Vec<Token> = Vec::new();
    let mut atomic = 0usize; // nesting depth inside a BEGIN ATOMIC body
    let toks = tokenize(sql);
    for (i, t) in toks.iter().enumerate() {
        if t.word == ";" && atomic == 0 {
            if let Some(hit) = check(&stmt) {
                return Some(hit);
            }
            stmt.clear();
            continue;
        }
        if t.word == "ATOMIC"
            && atomic == 0
            && i > 0
            && stmt.last().map(|p| p.word == "BEGIN").unwrap_or(false)
        {
            atomic = 1;
        } else if atomic > 0 && t.word == "CASE" {
            atomic += 1;
        } else if atomic > 0 && t.word == "END" {
            atomic -= 1;
        }
        stmt.push(t.clone());
    }
    check(&stmt)
}

fn check(stmt: &[Token]) -> Option<(String, usize)> {
    let first = stmt.first()?;
    let w = first.word.as_str();
    if TX_WORDS.contains(&w) {
        return Some((w.to_string(), first.line));
    }
    if (w == "START" || w == "PREPARE") && stmt.get(1).map(|t| t.word == "TRANSACTION").unwrap_or(false) {
        return Some((format!("{w} TRANSACTION"), first.line));
    }
    None
}

fn is_word_start(c: u8) -> bool {
    c == b'_' || c.is_ascii_alphabetic() || c >= 0x80
}

fn is_word_char(c: u8) -> bool {
    is_word_start(c) || c.is_ascii_digit() || c == b'$'
}

fn tokenize(src: &str) -> Vec<Token> {
    let s = src.as_bytes();
    let mut out = Vec::new();
    let mut line = 1usize;
    let mut i = 0usize;
    while i < s.len() {
        let c = s[i];
        if c == b'\n' {
            line += 1;
            i += 1;
        } else if c == b'-' && s.get(i + 1) == Some(&b'-') {
            while i < s.len() && s[i] != b'\n' {
                i += 1;
            }
        } else if c == b'/' && s.get(i + 1) == Some(&b'*') {
            let mut depth = 1;
            i += 2;
            while i < s.len() && depth > 0 {
                if s[i] == b'/' && s.get(i + 1) == Some(&b'*') {
                    depth += 1;
                    i += 2;
                } else if s[i] == b'*' && s.get(i + 1) == Some(&b'/') {
                    depth -= 1;
                    i += 2;
                } else {
                    if s[i] == b'\n' {
                        line += 1;
                    }
                    i += 1;
                }
            }
        } else if c == b'\'' || c == b'"' {
            (i, line) = skip_quoted(s, i, c, false, line);
        } else if c == b'$' {
            if let Some(end) = dollar_tag_end(s, i) {
                let tag = &s[i..end];
                match find(&s[end..], tag) {
                    Some(j) => {
                        line += s[end..end + j].iter().filter(|b| **b == b'\n').count();
                        i = end + j + tag.len();
                    }
                    None => i = s.len(), // unterminated: the server will reject it
                }
            } else {
                i += 1;
            }
        } else if c == b';' {
            out.push(Token { word: ";".into(), line });
            i += 1;
        } else if is_word_start(c) {
            let mut j = i + 1;
            while j < s.len() && is_word_char(s[j]) {
                j += 1;
            }
            if (c == b'E' || c == b'e') && j == i + 1 && s.get(j) == Some(&b'\'') {
                (i, line) = skip_quoted(s, j, b'\'', true, line);
                continue;
            }
            out.push(Token { word: String::from_utf8_lossy(&s[i..j]).to_uppercase(), line });
            i = j;
        } else {
            i += 1;
        }
    }
    out
}

fn find(hay: &[u8], needle: &[u8]) -> Option<usize> {
    hay.windows(needle.len()).position(|w| w == needle)
}

/// Skips a quoted run starting at s[i] (the opening quote); a doubled quote is an escaped quote;
/// with `backslash` (E'...') a backslash escapes the next byte. Returns the index after it.
fn skip_quoted(s: &[u8], mut i: usize, q: u8, backslash: bool, mut line: usize) -> (usize, usize) {
    i += 1;
    while i < s.len() {
        if backslash && s[i] == b'\\' && i + 1 < s.len() {
            if s[i + 1] == b'\n' {
                line += 1;
            }
            i += 2;
        } else if s[i] == q && s.get(i + 1) == Some(&q) {
            i += 2;
        } else if s[i] == q {
            return (i + 1, line);
        } else {
            if s[i] == b'\n' {
                line += 1;
            }
            i += 1;
        }
    }
    (i, line)
}

/// Index just past an opening $tag$ at s[i], or None ($1 is a parameter, not a quote).
fn dollar_tag_end(s: &[u8], i: usize) -> Option<usize> {
    let mut j = i + 1;
    if s.get(j) == Some(&b'$') {
        return Some(j + 1);
    }
    if j >= s.len() || !is_word_start(s[j]) || s[j] >= 0x80 {
        return None;
    }
    while j < s.len() && s[j] != b'$' {
        if !is_word_char(s[j]) {
            return None;
        }
        j += 1;
    }
    if j < s.len() {
        Some(j + 1)
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plain_ddl_passes() {
        assert!(find_tx_control("CREATE TABLE t (a int);\nCREATE INDEX i ON t(a);").is_none());
    }

    #[test]
    fn refuses_transaction_words() {
        for (sql, word, line) in [
            ("BEGIN;\nCREATE TABLE t(a int);", "BEGIN", 1),
            ("CREATE TABLE t(a int);\ncommit;", "COMMIT", 2),
            ("SELECT 1; START  TRANSACTION;", "START TRANSACTION", 1),
            ("SAVEPOINT s", "SAVEPOINT", 1),
        ] {
            assert_eq!(find_tx_control(sql), Some((word.to_string(), line)), "{sql}");
        }
    }

    #[test]
    fn ignores_comments_strings_and_dollar_bodies() {
        let sql = "-- BEGIN\n/* COMMIT */ SELECT 'COMMIT; BEGIN'; SELECT E'a\\'; COMMIT';\n\
                   DO $$ BEGIN PERFORM 1; END $$;\nCREATE FUNCTION f() RETURNS int LANGUAGE sql BEGIN ATOMIC SELECT 1; END;";
        assert!(find_tx_control(sql).is_none());
    }
}
