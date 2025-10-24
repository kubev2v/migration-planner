# Asynchronous RVTools Processing

This document demonstrates how to use the new asynchronous RVTools processing feature.

## Overview

The asynchronous processing system allows users to upload large RVTools files without waiting for the processing to complete. Instead, the API returns a job ID immediately, and clients can poll for the status.

## API Endpoints

### 1. Create Async Assessment
```
POST /api/v1/assessments/async
Content-Type: multipart/form-data

Form fields:
- name: Name of the assessment
- file: RVTools Excel file (binary)
```

**Response (202 Accepted):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "created_at": "2025-10-20T05:54:00Z"
}
```

### 2. Check Job Status
```
GET /api/v1/jobs/{job_id}
```

**Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "assessment_id": "550e8400-e29b-41d4-a716-446655440001",
  "created_at": "2025-10-20T05:54:00Z"
}
```

## Status Values

- `pending`: Job is queued for processing
- `running`: Job is currently being processed
- `completed`: Job completed successfully (assessment_id available)
- `failed`: Job failed (error field contains details)

## Usage Example

```bash
# 1. Upload RVTools file asynchronously
RESPONSE=$(curl -X POST "http://localhost:3443/api/v1/assessments/async" \
  -H "Authorization: Bearer $TOKEN" \
  -F "name=My RVTools Assessment" \
  -F "file=@rvtools-export.xlsx")

JOB_ID=$(echo $RESPONSE | jq -r '.id')
echo "Job created: $JOB_ID"

# 2. Poll for job status
while true; do
  STATUS_RESPONSE=$(curl -s "http://localhost:3443/api/v1/jobs/$JOB_ID" \
    -H "Authorization: Bearer $TOKEN")

  STATUS=$(echo $STATUS_RESPONSE | jq -r '.status')
  echo "Status: $STATUS"

  if [ "$STATUS" = "completed" ]; then
    ASSESSMENT_ID=$(echo $STATUS_RESPONSE | jq -r '.assessment_id')
    echo "Assessment created: $ASSESSMENT_ID"
    break
  elif [ "$STATUS" = "failed" ]; then
    ERROR=$(echo $STATUS_RESPONSE | jq -r '.error')
    echo "Job failed: $ERROR"
    break
  fi

  sleep 2
done

# 3. Get the completed assessment
curl "http://localhost:3443/api/v1/assessments/$ASSESSMENT_ID" \
  -H "Authorization: Bearer $TOKEN"
```

## Benefits

1. **Non-blocking**: Upload large files without timeout issues
2. **Better UX**: Users can perform other tasks while processing
3. **Scalable**: Background processing handles multiple concurrent uploads
4. **Resilient**: Jobs persist in memory and can be queried later

## Implementation Details

- Jobs are stored in memory (suitable for demo/development)
- Background goroutines handle the actual RVTools parsing
- Original synchronous endpoint (`POST /api/v1/assessments`) still works
- All existing RVTools parsing and OPA validation logic is reused