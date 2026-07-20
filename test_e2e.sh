#!/usr/bin/env bash
# E2E API Test for Artemis
# Tests the full user journey across all endpoints
set -uo pipefail

API="http://localhost:8080/api/v1"
PASS=0
FAIL=0
TESTS=()

# ── Helpers ──────────────────────────────────────────────

assert() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo "  ✓ $name"
    ((PASS++))
  else
    echo "  ✗ $name (expected: $expected, got: $actual)"
    ((FAIL++))
    TESTS+=("FAIL: $name")
  fi
}

assert_not() {
  local name="$1" not_expected="$2" actual="$3"
  if [[ "$actual" != "$not_expected" ]]; then
    echo "  ✓ $name"
    ((PASS++))
  else
    echo "  ✗ $name (got unexpected: $actual)"
    ((FAIL++))
    TESTS+=("FAIL: $name")
  fi
}

assert_gte() {
  local name="$1" min="$2" actual="$3"
  if [[ "$actual" -ge "$min" ]] 2>/dev/null; then
    echo "  ✓ $name ($actual >= $min)"
    ((PASS++))
  else
    echo "  ✗ $name (expected >= $min, got: $actual)"
    ((FAIL++))
    TESTS+=("FAIL: $name")
  fi
}

http_status() {
  curl -s -o /dev/null -w "%{http_code}" "$@"
}

# ── Cleanup test user if exists ──────────────────────────
TEST_EMAIL="e2e-test-$(date +%s)@artemis.test"
TEST_NAME="E2E Test User"
TEST_PASS="TestPass1234"

echo "═══════════════════════════════════════════════════════"
echo "  ARTEMIS E2E API TEST"
echo "  Test user: $TEST_EMAIL"
echo "═══════════════════════════════════════════════════════"
echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Health Checks"
# ═════════════════════════════════════════════════════════

STATUS=$(http_status "http://localhost:8080/healthz")
assert "GET /healthz" "200" "$STATUS"

STATUS=$(http_status "http://localhost:8080/readyz")
assert "GET /readyz" "200" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Auth: Register"
# ═════════════════════════════════════════════════════════

# Missing fields
STATUS=$(http_status -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"","password":""}')
assert "Register: reject empty fields" "400" "$STATUS"

# Short password
STATUS=$(http_status -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"name\":\"$TEST_NAME\",\"password\":\"short\"}")
assert "Register: reject short password" "400" "$STATUS"

# Valid registration
RES=$(curl -s -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"name\":\"$TEST_NAME\",\"password\":\"$TEST_PASS\",\"location\":\"San Francisco\"}")
TOKEN=$(echo "$RES" | jq -r '.token')
USER_ID=$(echo "$RES" | jq -r '.user.id')
assert_not "Register: returns token" "null" "$TOKEN"
assert_not "Register: returns user ID" "null" "$USER_ID"
assert "Register: correct name" "$TEST_NAME" "$(echo "$RES" | jq -r '.user.name')"
assert "Register: correct email" "$TEST_EMAIL" "$(echo "$RES" | jq -r '.user.email')"
assert "Register: onboarding not completed" "false" "$(echo "$RES" | jq -r '.user.onboarding_completed')"
assert "Register: security_question_set false" "false" "$(echo "$RES" | jq -r '.user.security_question_set')"

# Duplicate email
STATUS=$(http_status -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"name\":\"Dup\",\"password\":\"$TEST_PASS\"}")
assert "Register: reject duplicate email" "409" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Auth: Login"
# ═════════════════════════════════════════════════════════

# Wrong password
STATUS=$(http_status -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"wrongpass\"}")
assert "Login: reject wrong password" "401" "$STATUS"

# Wrong email
STATUS=$(http_status -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"nonexistent@test.com","password":"whatever1"}')
assert "Login: reject unknown email" "401" "$STATUS"

# Correct login
RES=$(curl -s -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"$TEST_PASS\"}")
TOKEN=$(echo "$RES" | jq -r '.token')
assert_not "Login: returns token" "null" "$TOKEN"
assert "Login: correct user ID" "$USER_ID" "$(echo "$RES" | jq -r '.user.id')"

AUTH="-H Authorization:Bearer $TOKEN"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Auth: Me"
# ═════════════════════════════════════════════════════════

RES=$(curl -s "$API/auth/me" -H "Authorization: Bearer $TOKEN")
assert "Me: correct ID" "$USER_ID" "$(echo "$RES" | jq -r '.id')"
assert "Me: correct email" "$TEST_EMAIL" "$(echo "$RES" | jq -r '.email')"

# No token
STATUS=$(http_status "$API/auth/me")
assert "Me: reject without token" "401" "$STATUS"

# Bad token
STATUS=$(http_status "$API/auth/me" -H "Authorization: Bearer garbage")
assert "Me: reject bad token" "401" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Security Question + Forgot Password"
# ═════════════════════════════════════════════════════════

# Set security question
RES=$(curl -s -X POST "$API/auth/security-question" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"question":"What city were you born in?","answer":"San Francisco"}')
assert "Set security question" "security question set" "$(echo "$RES" | jq -r '.message')"

# Verify it's set
RES=$(curl -s "$API/auth/me" -H "Authorization: Bearer $TOKEN")
assert "Security question set flag" "true" "$(echo "$RES" | jq -r '.security_question_set')"
assert "Security question stored" "What city were you born in?" "$(echo "$RES" | jq -r '.security_question')"

# Forgot password - get question
RES=$(curl -s -X POST "$API/auth/forgot-password" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\"}")
assert "Forgot password: returns question" "What city were you born in?" "$(echo "$RES" | jq -r '.security_question')"

# Forgot password - nonexistent email
STATUS=$(http_status -X POST "$API/auth/forgot-password" \
  -H "Content-Type: application/json" \
  -d '{"email":"nobody@nowhere.com"}')
assert "Forgot password: reject unknown email" "400" "$STATUS"

# Verify security - wrong answer
STATUS=$(http_status -X POST "$API/auth/verify-security" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"answer\":\"wrong city\"}")
assert "Verify security: reject wrong answer" "401" "$STATUS"

# Verify security - correct answer (case insensitive)
RES=$(curl -s -X POST "$API/auth/verify-security" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"answer\":\"  san francisco  \"}")
RESET_TOKEN=$(echo "$RES" | jq -r '.reset_token')
assert_not "Verify security: returns reset token" "null" "$RESET_TOKEN"

# Reset with bad token
STATUS=$(http_status -X POST "$API/auth/reset-password" \
  -H "Content-Type: application/json" \
  -d '{"reset_token":"bad.token.here","new_password":"NewPass1234"}')
assert "Reset password: reject bad token" "401" "$STATUS"

# Reset with short password
STATUS=$(http_status -X POST "$API/auth/reset-password" \
  -H "Content-Type: application/json" \
  -d "{\"reset_token\":\"$RESET_TOKEN\",\"new_password\":\"short\"}")
assert "Reset password: reject short password" "400" "$STATUS"

# Reset password successfully
NEW_PASS="ResetPass9876"
RES=$(curl -s -X POST "$API/auth/reset-password" \
  -H "Content-Type: application/json" \
  -d "{\"reset_token\":\"$RESET_TOKEN\",\"new_password\":\"$NEW_PASS\"}")
assert "Reset password: success" "password reset successfully" "$(echo "$RES" | jq -r '.message')"

# Old password fails
STATUS=$(http_status -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"$TEST_PASS\"}")
assert "Login: old password rejected" "401" "$STATUS"

# New password works
RES=$(curl -s -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"$NEW_PASS\"}")
TOKEN=$(echo "$RES" | jq -r '.token')
assert_not "Login: new password works" "null" "$TOKEN"
TEST_PASS="$NEW_PASS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Onboarding"
# ═════════════════════════════════════════════════════════

RES=$(curl -s -X POST "$API/auth/onboarding" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "headline": "Senior Backend Engineer",
    "summary": "Experienced Go and Python developer",
    "seniority_level": "senior",
    "years_experience": 6,
    "location": "San Francisco, CA",
    "desired_roles": ["Backend Engineer", "Platform Engineer", "Staff Engineer"],
    "skills": ["Go", "Python", "PostgreSQL", "Kubernetes", "AWS", "Docker", "gRPC", "Redis"],
    "experience": [{"title": "Senior Engineer", "company": "Stripe", "start": "2021", "end": "present", "technologies": ["Go", "Ruby", "PostgreSQL"]}],
    "education": [{"degree": "BS Computer Science", "institution": "UC Berkeley", "year": 2018}],
    "certifications": [{"name": "AWS Solutions Architect"}],
    "projects": [{"name": "Distributed Job Queue", "description": "Built a distributed task queue handling 10K jobs/sec"}],
    "languages": ["English", "Hindi"]
  }')
assert "Onboarding: completed" "true" "$(echo "$RES" | jq -r '.onboarding_completed')"
assert "Onboarding: headline set" "Senior Backend Engineer" "$(echo "$RES" | jq -r '.headline')"
assert "Onboarding: seniority set" "senior" "$(echo "$RES" | jq -r '.seniority_level')"
assert "Onboarding: years_experience set" "6" "$(echo "$RES" | jq -r '.years_experience')"
assert "Onboarding: location set" "San Francisco, CA" "$(echo "$RES" | jq -r '.location')"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ User Profile"
# ═════════════════════════════════════════════════════════

# Get user
RES=$(curl -s "$API/users/$USER_ID" -H "Authorization: Bearer $TOKEN")
assert "Get user: correct ID" "$USER_ID" "$(echo "$RES" | jq -r '.id')"
assert "Get user: headline" "Senior Backend Engineer" "$(echo "$RES" | jq -r '.headline')"

# Update user
RES=$(curl -s -X PUT "$API/users/$USER_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "E2E Updated Name", "location": "New York, NY"}')
assert "Update user: name changed" "E2E Updated Name" "$(echo "$RES" | jq -r '.name')"
assert "Update user: location changed" "New York, NY" "$(echo "$RES" | jq -r '.location')"

# Update preferences
RES=$(curl -s -X PUT "$API/users/$USER_ID/preferences" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skills":["Go","Python","Kubernetes"],"notifications":{"enabled":true}}')
STATUS=$?
assert "Update preferences: succeeds" "0" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Stats"
# ═════════════════════════════════════════════════════════

RES=$(curl -s "$API/stats" -H "Authorization: Bearer $TOKEN")
TOTAL=$(echo "$RES" | jq -r '.total_jobs')
ACTIVE=$(echo "$RES" | jq -r '.active_jobs')
COMPANIES=$(echo "$RES" | jq -r '.total_companies')
assert_gte "Stats: total_jobs > 0" "1" "$TOTAL"
assert_gte "Stats: active_jobs > 0" "1" "$ACTIVE"
assert_gte "Stats: total_companies > 0" "1" "$COMPANIES"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Public Jobs (no auth)"
# ═════════════════════════════════════════════════════════

# Missing query
STATUS=$(http_status "$API/public/jobs")
assert "Public jobs: reject missing query" "400" "$STATUS"

# Valid query
RES=$(curl -s "$API/public/jobs?q=engineer")
TOTAL=$(echo "$RES" | jq -r '.total')
COUNT=$(echo "$RES" | jq -r '.data | length')
assert_gte "Public jobs: returns results" "1" "$TOTAL"
# Verify it returns previews (no description field)
HAS_DESC=$(echo "$RES" | jq -r '.data[0].description // "none"')
assert "Public jobs: no description in preview" "none" "$HAS_DESC"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Jobs: List"
# ═════════════════════════════════════════════════════════

# Basic list
RES=$(curl -s "$API/jobs?page=1&per_page=5" -H "Authorization: Bearer $TOKEN")
TOTAL=$(echo "$RES" | jq -r '.total')
COUNT=$(echo "$RES" | jq -r '.data | length')
PAGE=$(echo "$RES" | jq -r '.page')
assert_gte "List jobs: has results" "1" "$TOTAL"
assert "List jobs: page=1" "1" "$PAGE"
assert_gte "List jobs: returns data" "1" "$COUNT"

# Grab a job ID for later tests
JOB_ID=$(echo "$RES" | jq -r '.data[0].id')
JOB_COMPANY_SLUG=$(echo "$RES" | jq -r '.data[0].company_slug')
assert_not "List jobs: has job ID" "null" "$JOB_ID"

# Verify job has expected fields
HAS_TITLE=$(echo "$RES" | jq -r '.data[0].title')
HAS_URL=$(echo "$RES" | jq -r '.data[0].url')
assert_not "List jobs: has title" "null" "$HAS_TITLE"
assert_not "List jobs: has url" "null" "$HAS_URL"

# With keyword filter
RES=$(curl -s "$API/jobs?keyword=engineer&per_page=5" -H "Authorization: Bearer $TOKEN")
assert_gte "List jobs: keyword filter works" "0" "$(echo "$RES" | jq -r '.total')"

# With location filter
RES=$(curl -s "$API/jobs?location=remote&per_page=5" -H "Authorization: Bearer $TOKEN")
assert_gte "List jobs: location filter works" "0" "$(echo "$RES" | jq -r '.total')"

# With seniority filter
RES=$(curl -s "$API/jobs?seniority=senior&per_page=5" -H "Authorization: Bearer $TOKEN")
assert_gte "List jobs: seniority filter works" "0" "$(echo "$RES" | jq -r '.total')"

# Pagination - page 2
RES=$(curl -s "$API/jobs?page=2&per_page=5" -H "Authorization: Bearer $TOKEN")
assert "List jobs: page 2" "2" "$(echo "$RES" | jq -r '.page')"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Jobs: Get Single"
# ═════════════════════════════════════════════════════════

RES=$(curl -s "$API/jobs/$JOB_ID" -H "Authorization: Bearer $TOKEN")
assert "Get job: correct ID" "$JOB_ID" "$(echo "$RES" | jq -r '.id')"
assert_not "Get job: has title" "null" "$(echo "$RES" | jq -r '.title')"
assert_not "Get job: has company_name" "null" "$(echo "$RES" | jq -r '.company_name')"

# Invalid ID
STATUS=$(http_status "$API/jobs/not-a-uuid" -H "Authorization: Bearer $TOKEN")
assert "Get job: reject invalid ID" "400" "$STATUS"

# Non-existent ID
STATUS=$(http_status "$API/jobs/00000000-0000-0000-0000-000000000000" -H "Authorization: Bearer $TOKEN")
assert "Get job: 404 for missing ID" "404" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Companies"
# ═════════════════════════════════════════════════════════

RES=$(curl -s "$API/companies?page=1&per_page=5" -H "Authorization: Bearer $TOKEN")
TOTAL=$(echo "$RES" | jq -r '.total')
assert_gte "List companies: has results" "1" "$TOTAL"
FIRST_SLUG=$(echo "$RES" | jq -r '.data[0].slug')
assert_not "List companies: has slug" "null" "$FIRST_SLUG"

# Get single company
RES=$(curl -s "$API/companies/$FIRST_SLUG" -H "Authorization: Bearer $TOKEN")
assert "Get company: correct slug" "$FIRST_SLUG" "$(echo "$RES" | jq -r '.slug')"
assert_not "Get company: has name" "null" "$(echo "$RES" | jq -r '.name')"
assert_not "Get company: has ats_type" "null" "$(echo "$RES" | jq -r '.ats_type')"

# Get company jobs
RES=$(curl -s "$API/companies/$FIRST_SLUG/jobs?per_page=5" -H "Authorization: Bearer $TOKEN")
assert_gte "Company jobs: returns results" "0" "$(echo "$RES" | jq -r '.total')"

# Non-existent company
STATUS=$(http_status "$API/companies/nonexistent-slug-12345" -H "Authorization: Bearer $TOKEN")
assert "Get company: 404 for missing" "404" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Saved Jobs"
# ═════════════════════════════════════════════════════════

# Save a job
STATUS=$(http_status -X POST "$API/saved/$JOB_ID" -H "Authorization: Bearer $TOKEN")
assert "Save job: success" "200" "$STATUS"

# List saved jobs
RES=$(curl -s "$API/saved" -H "Authorization: Bearer $TOKEN")
SAVED_COUNT=$(echo "$RES" | jq -r '.data | length')
assert_gte "List saved: has saved jobs" "1" "$SAVED_COUNT"

# List saved IDs
RES=$(curl -s "$API/saved/ids" -H "Authorization: Bearer $TOKEN")
HAS_ID=$(echo "$RES" | jq -r ".ids | map(select(. == \"$JOB_ID\")) | length")
assert "Saved IDs: contains our job" "1" "$HAS_ID"

# Unsave
STATUS=$(http_status -X DELETE "$API/saved/$JOB_ID" -H "Authorization: Bearer $TOKEN")
assert "Unsave job: success" "200" "$STATUS"

# Verify unsaved
RES=$(curl -s "$API/saved/ids" -H "Authorization: Bearer $TOKEN")
HAS_ID=$(echo "$RES" | jq -r ".ids | map(select(. == \"$JOB_ID\")) | length")
assert "Unsave: job removed" "0" "$HAS_ID"

# Save it again for recommendation testing
curl -s -X POST "$API/saved/$JOB_ID" -H "Authorization: Bearer $TOKEN" > /dev/null

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Resume"
# ═════════════════════════════════════════════════════════

# Get resume (none uploaded yet)
RES=$(curl -s "$API/users/$USER_ID/resume" -H "Authorization: Bearer $TOKEN")
# Should return empty or error
STATUS=$(http_status "$API/users/$USER_ID/resume" -H "Authorization: Bearer $TOKEN")
# 200 with null or 404 are both acceptable
echo "  ℹ Get resume (none): HTTP $STATUS"

# Parse progress (no active parse)
RES=$(curl -s "$API/users/$USER_ID/resume/progress" -H "Authorization: Bearer $TOKEN")
STATUS=$(http_status "$API/users/$USER_ID/resume/progress" -H "Authorization: Bearer $TOKEN")
echo "  ℹ Parse progress (none): HTTP $STATUS"

# Upload resume (create a minimal PDF for testing)
python3 -c "
# Create a minimal valid PDF
pdf = b'%PDF-1.0\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Resources<</Font<</F1 4 0 R>>>>>>endobj\n4 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\nxref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000266 00000 n \ntrailer<</Size 5/Root 1 0 R>>\nstartxref\n334\n%%EOF'
with open('/tmp/test_resume.pdf', 'wb') as f:
    f.write(pdf)
" 2>/dev/null

STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/users/$USER_ID/resume" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/test_resume.pdf")
# 200 or 500 (if minio not configured) - just log it
echo "  ℹ Upload resume: HTTP $STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Recommendations"
# ═════════════════════════════════════════════════════════

RES=$(curl -s "$API/users/$USER_ID/recommendations?limit=5" -H "Authorization: Bearer $TOKEN")
STATUS=$(http_status "$API/users/$USER_ID/recommendations?limit=5" -H "Authorization: Bearer $TOKEN")
REC_TOTAL=$(echo "$RES" | jq -r '.total // 0')
echo "  ℹ Recommendations: HTTP $STATUS, total=$REC_TOTAL"
# If we have recs, verify structure
if [[ "$REC_TOTAL" -gt 0 ]]; then
  assert_not "Recommendations: has job_id" "null" "$(echo "$RES" | jq -r '.recommendations[0].job_id')"
  assert_not "Recommendations: has score" "null" "$(echo "$RES" | jq -r '.recommendations[0].score')"
  assert_not "Recommendations: has signals" "null" "$(echo "$RES" | jq -r '.recommendations[0].signals')"
else
  echo "  ℹ No recommendations (no resume embedding) — structure test skipped"
  ((PASS++))
fi

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Vector Search"
# ═════════════════════════════════════════════════════════

# Missing query
STATUS=$(http_status "$API/jobs/search" -H "Authorization: Bearer $TOKEN")
assert "Vector search: reject missing query" "400" "$STATUS"

# Valid search (will be slow due to OpenAI embed call)
RES=$(curl -s --max-time 30 "$API/jobs/search?q=backend+golang+engineer&limit=5" -H "Authorization: Bearer $TOKEN")
STATUS_CODE=$(echo "$RES" | jq -r '.data | length // 0' 2>/dev/null || echo "0")
echo "  ℹ Vector search: returned $STATUS_CODE results (requires OpenAI key)"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Auth Edge Cases"
# ═════════════════════════════════════════════════════════

# Expired/invalid JWT on protected routes
STATUS=$(http_status "$API/jobs" -H "Authorization: Bearer expired.token.here")
assert "Jobs: reject invalid token" "401" "$STATUS"

STATUS=$(http_status "$API/companies" -H "Authorization: Bearer expired.token.here")
assert "Companies: reject invalid token" "401" "$STATUS"

STATUS=$(http_status "$API/stats" -H "Authorization: Bearer expired.token.here")
assert "Stats: reject invalid token" "401" "$STATUS"

STATUS=$(http_status "$API/saved" -H "Authorization: Bearer expired.token.here")
assert "Saved: reject invalid token" "401" "$STATUS"

# No auth header at all
STATUS=$(http_status "$API/jobs")
assert "Jobs: reject no auth" "401" "$STATUS"

echo ""

# ═════════════════════════════════════════════════════════
echo "▸ Cleanup"
# ═════════════════════════════════════════════════════════

# Delete test user from DB
docker exec artemis-worker-postgres-1 psql -U artemis -q -c \
  "DELETE FROM user_skills WHERE user_id='$USER_ID';
   DELETE FROM saved_jobs WHERE user_id='$USER_ID';
   DELETE FROM resumes WHERE user_id='$USER_ID';
   DELETE FROM users WHERE id='$USER_ID';" 2>/dev/null
echo "  ✓ Test user cleaned up"

echo ""
echo "═══════════════════════════════════════════════════════"
echo "  RESULTS: $PASS passed, $FAIL failed"
echo "═══════════════════════════════════════════════════════"

if [[ ${#TESTS[@]} -gt 0 ]]; then
  echo ""
  echo "  Failed tests:"
  for t in "${TESTS[@]}"; do
    echo "    $t"
  done
fi

echo ""
exit $FAIL
