#!/usr/bin/env bash

# Safe, deliberately small dotenv reader for the local lifecycle scripts.
#
# Do not replace this with `source .env`: DATABASE_URL, cookies and tokens are
# data, not shell programs.  Values are exported through `export "KEY=value"`,
# where KEY is validated before it is used.

trim_env_whitespace() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

parse_env_assignment() {
  local line="$1" name value

  line="$(trim_env_whitespace "$line")"
  [ -n "$line" ] || return 1
  case "$line" in
    \#*) return 1 ;;
    export[[:space:]]*) line="$(trim_env_whitespace "${line#export}")" ;;
  esac

  if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$ ]]; then
    name="${BASH_REMATCH[1]}"
    value="$(trim_env_whitespace "${BASH_REMATCH[2]}")"
  else
    return 1
  fi

  # A quoted value is taken literally, without evaluating command
  # substitutions, arithmetic, backticks or escape sequences. Allow a
  # trailing dotenv comment after the closing quote; this is important for
  # cookie values that themselves contain '#'.
  # Keep the regular expressions in variables. A literal single quote in
  # the right-hand side of [[ ... =~ ... ]] is otherwise consumed by Bash's
  # parser, making the single-quoted branch accidentally match unquoted data.
  local double_quoted_re='^"(.*)"[[:space:]]*(#.*)?$'
  local single_quoted_re="^'(.*)'[[:space:]]*(#.*)?$"
  if [[ "$value" =~ $double_quoted_re ]]; then
    value="${BASH_REMATCH[1]}"
  elif [[ "$value" =~ $single_quoted_re ]]; then
    value="${BASH_REMATCH[1]}"
  else
    # For unquoted dotenv values, an inline comment starts at '#' only when
    # it is preceded by whitespace. A URL fragment or cookie containing '#'
    # is therefore preserved.
    value="${value%%[[:space:]]#*}"
    value="$(trim_env_whitespace "$value")"
  fi

  ENV_ASSIGNMENT_NAME="$name"
  ENV_ASSIGNMENT_VALUE="$value"
  return 0
}

env_file_value() {
  local key="$1" file="$2" line
  [ -n "$key" ] || return 2
  [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || return 2

  # Explicit process environment wins over the local file, matching normal
  # deployment expectations and allowing CI/launchd overrides.
  if [ "${!key+x}" = x ]; then
    printf '%s' "${!key}"
    return 0
  fi
  [ -f "$file" ] || return 0

  while IFS= read -r line || [ -n "$line" ]; do
    if parse_env_assignment "$line" && [ "$ENV_ASSIGNMENT_NAME" = "$key" ]; then
      printf '%s' "$ENV_ASSIGNMENT_VALUE"
      return 0
    fi
  done < "$file"
}

load_env_file() {
  local file="$1" line
  [ -f "$file" ] || return 1

  while IFS= read -r line || [ -n "$line" ]; do
    if parse_env_assignment "$line"; then
      export "$ENV_ASSIGNMENT_NAME=$ENV_ASSIGNMENT_VALUE"
    fi
  done < "$file"
}

env_file_has_key() {
  local key="$1" file="$2" line
  [ -f "$file" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    if parse_env_assignment "$line" && [ "$ENV_ASSIGNMENT_NAME" = "$key" ]; then
      return 0
    fi
  done < "$file"
  return 1
}
