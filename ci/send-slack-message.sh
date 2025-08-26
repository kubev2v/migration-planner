#!/usr/bin/env bash

# This script sends Slack messages with a title and detailed changes.
# It supports both Slack webhooks and Web API for maximum flexibility.
#
# THREADING SUPPORT:
# - Slack Web API (SLACK_BOT_TOKEN + SLACK_CHANNEL): Full threading support
# - Slack webhooks (SLACK_WEBHOOK_URL): Separate messages (no threading)
#
# The repository name is automatically extracted from git remote URL or directory name.
#
# Usage:
#   ./ci/send-slack-message.sh "message_type_part" "change_details"
#
# Example:
#   ./ci/send-slack-message.sh "New Tag Release: v1.2.3" "• Feature: Added new API\n• Fix: Resolved bug #123"
#   ./ci/send-slack-message.sh "Weekly Summary (Last 7 Days) 📅" "• Feature: Added new API\n• Fix: Resolved bug #123"
#
# Environment variables (one of these is required):
#   SLACK_WEBHOOK_URL - Slack webhook URL for sending messages (no threading support)
#   SLACK_BOT_TOKEN + SLACK_CHANNEL - Slack bot token and channel for Web API (supports threading)

set -euo pipefail

# Check required arguments
if [ $# -ne 2 ]; then
  echo "Usage: $0 \"message_type_part\" \"change_details\"" >&2
  echo "Example: $0 \"New Tag Release: v1.2.3\" \"change details here\"" >&2
  exit 1
fi

# Check required environment variables
if [ -n "${SLACK_BOT_TOKEN:-}" ] && [ -n "${SLACK_CHANNEL:-}" ]; then
  USE_WEB_API=true
  echo "Using Slack Web API (supports threading)"
elif [ -n "${SLACK_WEBHOOK_URL:-}" ]; then
  USE_WEB_API=false
  echo "Using Slack webhook (no threading support)"
else
  echo "Error: Either SLACK_WEBHOOK_URL or (SLACK_BOT_TOKEN + SLACK_CHANNEL) must be set" >&2
  echo "For threading support, use: SLACK_BOT_TOKEN and SLACK_CHANNEL" >&2
  echo "For simple messaging, use: SLACK_WEBHOOK_URL" >&2
  exit 1
fi

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
  echo "Error: Not in a git repository" >&2
  exit 1
fi

# Check if jq is available
if ! command -v jq &> /dev/null; then
  echo "Error: jq is required but not installed." >&2
  exit 1
fi

MESSAGE_TYPE="$1"
CHANGE_DETAILS="$2"

# Get repository name from git remote URL and current date
REMOTE_URL=$(git config --get remote.origin.url 2>/dev/null || echo "")
if [ -n "$REMOTE_URL" ]; then
  # Extract repo name from various URL formats:
  # https://github.com/owner/repo.git -> repo
  # git@github.com:owner/repo.git -> repo  
  # https://github.com/owner/repo -> repo
  REPO_NAME=$(echo "$REMOTE_URL" | sed -E 's|.*[:/]([^/]+)/([^/]+)/?(.git)?$|\2|' | sed 's/\.git$//')
else
  # Fallback: use directory name if no remote
  REPO_NAME=$(basename "$(git rev-parse --show-toplevel)")
fi

CURRENT_DATE=$(date +"%Y-%m-%d")

if [ "$USE_WEB_API" = true ]; then
  # Web API: Send title with thread icon, then details in thread
  TITLE_MESSAGE="*${REPO_NAME}* - ${CURRENT_DATE} - ${MESSAGE_TYPE} 🧵"
  
  echo "Sending title message with threading support..."
  
  TITLE_PAYLOAD=$(jq -n --arg channel "$SLACK_CHANNEL" --arg msg "$TITLE_MESSAGE" '{"channel": $channel, "text": $msg}')
  RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
    -H 'Content-type: application/json' \
    --data "$TITLE_PAYLOAD" \
    "https://slack.com/api/chat.postMessage")
  
  echo "Slack API response: $RESPONSE"
  
  # Check if the API call was successful
  if echo "$RESPONSE" | jq -r '.ok' | grep -q "true"; then
    TIMESTAMP=$(echo "$RESPONSE" | jq -r '.ts')
    echo "Title message sent successfully. Timestamp: $TIMESTAMP"
  else
    ERROR_MSG=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
    echo "Error sending message: $ERROR_MSG" >&2
    exit 1
  fi
else
  # Webhook: Send title + body in one message
  TITLE_MESSAGE="*${REPO_NAME}* - ${CURRENT_DATE} - ${MESSAGE_TYPE}"
  
  # Check if we have change details to include
  if [ -n "$CHANGE_DETAILS" ]; then
    COMBINED_MESSAGE="${TITLE_MESSAGE}"$'\n'$'\n'"${CHANGE_DETAILS}"
  else
    COMBINED_MESSAGE="$TITLE_MESSAGE"
  fi
  
  echo "Sending combined message via webhook..."
  
  COMBINED_PAYLOAD=$(jq -n --arg msg "$COMBINED_MESSAGE" '{"text": $msg}')
  RESPONSE=$(curl -s -X POST -H 'Content-type: application/json' --data "$COMBINED_PAYLOAD" "$SLACK_WEBHOOK_URL")
  
  echo "Webhook response: $RESPONSE"
  echo "Combined message sent via webhook."
  
  # Exit here since webhook sends everything in one message
  echo "Slack notification completed."
  exit 0
fi

# Check if we have change details to send
if [ -z "$CHANGE_DETAILS" ]; then
  echo "No change details provided, skipping thread message."
  exit 0
fi

if [ "$USE_WEB_API" = true ] && [ -n "$TIMESTAMP" ]; then
  echo "Sending details as threaded reply..."
  THREAD_PAYLOAD=$(jq -n --arg channel "$SLACK_CHANNEL" --arg msg "$CHANGE_DETAILS" --arg ts "$TIMESTAMP" '{"channel": $channel, "text": $msg, "thread_ts": $ts}')
  THREAD_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
    -H 'Content-type: application/json' \
    --data "$THREAD_PAYLOAD" \
    "https://slack.com/api/chat.postMessage")
  
  if echo "$THREAD_RESPONSE" | jq -r '.ok' | grep -q "true"; then
    echo "Threaded message sent successfully."
  else
    ERROR_MSG=$(echo "$THREAD_RESPONSE" | jq -r '.error // "Unknown error"')
    echo "Error sending threaded message: $ERROR_MSG" >&2
    exit 1
  fi
else
  echo "Sending details as separate message..."
  if [ "$USE_WEB_API" = true ]; then
    # Use Web API for separate message
    DETAILS_PAYLOAD=$(jq -n --arg channel "$SLACK_CHANNEL" --arg msg "$CHANGE_DETAILS" '{"channel": $channel, "text": $msg}')
    DETAILS_RESPONSE=$(curl -s -X POST \
      -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
      -H 'Content-type: application/json' \
      --data "$DETAILS_PAYLOAD" \
      "https://slack.com/api/chat.postMessage")
    
    if echo "$DETAILS_RESPONSE" | jq -r '.ok' | grep -q "true"; then
      echo "Details message sent successfully."
    else
      ERROR_MSG=$(echo "$DETAILS_RESPONSE" | jq -r '.error // "Unknown error"')
      echo "Error sending details message: $ERROR_MSG" >&2
      exit 1
    fi
  else
    # Use webhook for separate message
    DETAILS_PAYLOAD=$(jq -n --arg msg "$CHANGE_DETAILS" '{"text": $msg}')
    curl -s -X POST -H 'Content-type: application/json' --data "$DETAILS_PAYLOAD" "$SLACK_WEBHOOK_URL"
    echo "Details message sent via webhook."
  fi
fi

echo "Slack notification completed."
