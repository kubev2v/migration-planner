#!/usr/bin/env bash

# This script generates a formatted Slack message from a list of commit messages.
# It takes the commit log and author as input and outputs a JSON payload.
#
# Requirements:
# - Bash 4.0+ (for associative arrays)
# - jq (for JSON formatting)
#
# Setup for cross-platform compatibility:
# On Linux: Usually has bash 4+ by default
# On macOS: Run `brew install bash` then add to PATH:
#   echo 'export PATH="/opt/homebrew/bin:$PATH"' >> ~/.zshrc
#   source ~/.zshrc
#
# Usage:
#   git log --pretty=format:"%an|%s" | ./ci/create-slack-message.sh

set -euo pipefail

# Check if we have bash 4.0+ for associative arrays
if [ "${BASH_VERSION%%.*}" -lt 4 ]; then
  echo "Error: This script requires Bash 4.0 or later for associative arrays." >&2
  echo "Current version: $BASH_VERSION" >&2
  echo "Please ensure you have bash 4+ in your PATH." >&2
  echo "On macOS: brew install bash && export PATH=\"/opt/homebrew/bin:\$PATH\"" >&2
  exit 1
fi

if [ -t 0 ]; then
  echo "Error: This script expects commit messages via stdin." >&2
  exit 1
fi

# Check if jq is available
if ! command -v jq &> /dev/null; then
  echo "Error: jq is required but not installed." >&2
  exit 1
fi

declare -A COMMITS_BY_TYPE
COMMITS_BY_TYPE[feat]=""
COMMITS_BY_TYPE[fix]=""
COMMITS_BY_TYPE[build]=""
COMMITS_BY_TYPE[ci]=""
COMMITS_BY_TYPE[docs]=""
COMMITS_BY_TYPE[perf]=""
COMMITS_BY_TYPE[refactor]=""
COMMITS_BY_TYPE[style]=""
COMMITS_BY_TYPE[test]=""
COMMITS_BY_TYPE[misc]=""
COMMITS_BY_TYPE[bot]=""

# Track JIRA tickets to avoid duplicates
declare -A JIRA_TICKETS_SEEN
declare -A JIRA_TICKET_TYPES
declare -A JIRA_TICKET_MESSAGES

# Define commit type priority (higher number = higher priority)
declare -A TYPE_PRIORITY
TYPE_PRIORITY[feat]=9
TYPE_PRIORITY[fix]=8
TYPE_PRIORITY[perf]=7
TYPE_PRIORITY[refactor]=6
TYPE_PRIORITY[build]=5
TYPE_PRIORITY[ci]=4
TYPE_PRIORITY[docs]=3
TYPE_PRIORITY[test]=2
TYPE_PRIORITY[style]=1
TYPE_PRIORITY[misc]=0

# Define a list of bot accounts to ignore commit parsing for
BOT_USERS=("github-actions[bot]" "dependabot[bot]" "red-hat-konflux[bot]")

is_bot_user() {
  local user="$1"
  for bot in "${BOT_USERS[@]}"; do
    if [[ "$user" == "$bot" ]]; then
      return 0 # is a bot
    fi
  done
  return 1 # not a bot
}

while IFS=$'\n' read -r line; do
  AUTHOR=$(echo "$line" | cut -d'|' -f1 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  COMMIT_SUBJECT=$(echo "$line" | cut -d'|' -f2- | sed 's/^[[:space:]]*//')

  if is_bot_user "$AUTHOR"; then
    FORMATTED_COMMIT="• $COMMIT_SUBJECT"
    COMMITS_BY_TYPE[bot]+="$FORMATTED_COMMIT"$'\n'
  else
    JIRA_TICKET=$(echo "$COMMIT_SUBJECT" | grep -o -E '^\w+-\d+' || echo "NO-JIRA")
    
    # Extract commit type - look for pattern "type:" in commit message
    if echo "$COMMIT_SUBJECT" | grep -q -iE '(feat|fix|docs|style|refactor|perf|test|build|ci):'; then
      COMMIT_TYPE=$(echo "$COMMIT_SUBJECT" | grep -oiE '(feat|fix|docs|style|refactor|perf|test|build|ci):' | sed 's/://' | tr '[:upper:]' '[:lower:]')
      # Extract everything after "type: " case-insensitively
      COMMIT_MESSAGE=$(echo "$COMMIT_SUBJECT" | sed -E 's/.*(feat|fix|docs|style|refactor|perf|test|build|ci): *(.*)/\2/i')
    else
      COMMIT_TYPE="misc"
      COMMIT_MESSAGE="$COMMIT_SUBJECT"
    fi

    # Handle JIRA tickets - group commits by ticket with priority-based type selection
    if [ "$JIRA_TICKET" != "NO-JIRA" ]; then
      # Check if we've already seen this JIRA ticket
      if [[ ! -v JIRA_TICKETS_SEEN[$JIRA_TICKET] ]]; then
        # First time seeing this ticket
        JIRA_TICKETS_SEEN[$JIRA_TICKET]=1
        JIRA_TICKET_TYPES[$JIRA_TICKET]=$COMMIT_TYPE
        JIRA_TICKET_MESSAGES[$JIRA_TICKET]="$COMMIT_MESSAGE"
      else
        # We've seen this ticket before - check if current type has higher priority
        CURRENT_PRIORITY=${TYPE_PRIORITY[$COMMIT_TYPE]:-0}
        EXISTING_TYPE=${JIRA_TICKET_TYPES[$JIRA_TICKET]}
        EXISTING_PRIORITY=${TYPE_PRIORITY[$EXISTING_TYPE]:-0}
        
        if [ "$CURRENT_PRIORITY" -gt "$EXISTING_PRIORITY" ]; then
          # Update to higher priority type
          JIRA_TICKET_TYPES[$JIRA_TICKET]=$COMMIT_TYPE
          JIRA_TICKET_MESSAGES[$JIRA_TICKET]="$COMMIT_MESSAGE"
        fi
      fi
    else
      # No JIRA ticket - add commit normally
      FORMATTED_COMMIT="• $COMMIT_MESSAGE"
      if [[ -v COMMITS_BY_TYPE[$COMMIT_TYPE] ]]; then
        COMMITS_BY_TYPE[$COMMIT_TYPE]+="$FORMATTED_COMMIT"$'\n'
      else
        COMMITS_BY_TYPE[misc]+="$FORMATTED_COMMIT"$'\n'
      fi
    fi
  fi
done

# Second pass: Process all collected JIRA tickets
for JIRA_TICKET in "${!JIRA_TICKETS_SEEN[@]}"; do
  COMMIT_TYPE=${JIRA_TICKET_TYPES[$JIRA_TICKET]}
  COMMIT_MESSAGE=${JIRA_TICKET_MESSAGES[$JIRA_TICKET]}
  
  # Try to fetch JIRA ticket title if API is available
  JIRA_TITLE=""
  if [ -n "${JIRA_BASE_URL:-}" ] && [ -n "${JIRA_API_TOKEN:-}" ]; then
    JIRA_TITLE=$(curl -s -H "Authorization: Bearer $JIRA_API_TOKEN" \
      "$JIRA_BASE_URL/rest/api/2/issue/$JIRA_TICKET" | \
      jq -r '.fields.summary // empty' 2>/dev/null || echo "")
  fi
  
  # Use JIRA title if available, otherwise use commit message
  if [ -n "$JIRA_TITLE" ]; then
    DISPLAY_MESSAGE="$JIRA_TITLE"
  else
    DISPLAY_MESSAGE="$COMMIT_MESSAGE"
  fi
  
  if [ -n "${JIRA_BASE_URL:-}" ]; then
    JIRA_LINK="${JIRA_BASE_URL}/browse/$JIRA_TICKET"
    FORMATTED_COMMIT="• $DISPLAY_MESSAGE [<$JIRA_LINK|$JIRA_TICKET>]"
  else
    FORMATTED_COMMIT="• $DISPLAY_MESSAGE [$JIRA_TICKET]"
  fi
  
  if [[ -v COMMITS_BY_TYPE[$COMMIT_TYPE] ]]; then
    COMMITS_BY_TYPE[$COMMIT_TYPE]+="$FORMATTED_COMMIT"$'\n'
  else
    COMMITS_BY_TYPE[misc]+="$FORMATTED_COMMIT"$'\n'
  fi
done

MESSAGE_TEXT=""

[ ! -z "${COMMITS_BY_TYPE[feat]}" ] && MESSAGE_TEXT+=$'\n'":sparkles: *New Features*"$'\n'"${COMMITS_BY_TYPE[feat]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[fix]}" ] && MESSAGE_TEXT+=$'\n'":bug: *Bug Fixes*"$'\n'"${COMMITS_BY_TYPE[fix]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[perf]}" ] && MESSAGE_TEXT+=$'\n'":rocket: *Performance Improvements*"$'\n'"${COMMITS_BY_TYPE[perf]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[refactor]}" ] && MESSAGE_TEXT+=$'\n'":recycle: *Refactoring*"$'\n'"${COMMITS_BY_TYPE[refactor]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[docs]}" ] && MESSAGE_TEXT+=$'\n'":memo: *Documentation*"$'\n'"${COMMITS_BY_TYPE[docs]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[test]}" ] && MESSAGE_TEXT+=$'\n'":white_check_mark: *Tests*"$'\n'"${COMMITS_BY_TYPE[test]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[build]}" ] && MESSAGE_TEXT+=$'\n'":package: *Build System & Dependencies*"$'\n'"${COMMITS_BY_TYPE[build]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[ci]}" ] && MESSAGE_TEXT+=$'\n'":gear: *CI Changes*"$'\n'"${COMMITS_BY_TYPE[ci]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[style]}" ] && MESSAGE_TEXT+=$'\n'":nail_care: *Code Style Changes*"$'\n'"${COMMITS_BY_TYPE[style]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[misc]}" ] && MESSAGE_TEXT+=$'\n'":file_folder: *Other Changes*"$'\n'"${COMMITS_BY_TYPE[misc]}"$'\n'
[ ! -z "${COMMITS_BY_TYPE[bot]}" ] && MESSAGE_TEXT+=$'\n'":robot_face: *Bot Commits*"$'\n'"${COMMITS_BY_TYPE[bot]}"$'\n'

jq -n --arg msg "$MESSAGE_TEXT" '{"text": $msg}'
