---
name: blind-spot-coverage
description: Use this skill when the user wants to add test coverage targeting blind spots in a specific method — uncovered branches, edge cases, or unusual inputs. Activate on explicit requests to cover blind spots or when the user asks "what should I test here?" about a particular method. This is not about achieving 100% line coverage, but about ensuring the method behaves correctly in scenarios that are easy to miss.
---

# Blind Spot Coverage

Activate when the user wants to identify and cover blind spots in a specific method's test coverage. The goal is to find what the existing tests *don't* cover — the edge cases, error paths, and unusual inputs that could break the method.

## Input

The user provides:
- **Target method** (required): The specific method/function to analyze (e.g., `UserService.validateEmail`, `parseConfig`)
- **Context** (optional): Any specific concerns or areas they want to focus on

Example: `/blind-spot-coverage UserService.validateEmail`

## Procedure

### 1. Locate the method

Find the method in the codebase. Read it thoroughly, including:
- The method signature and return type
- All parameters and their types
- The full implementation body
- Any helper methods it calls internally

### 2. Find existing tests

Locate all existing tests for this method. Look for:
- Test files with matching names (e.g., `UserService.test.ts`, `test_user_service.py`)
- Test cases that explicitly call this method
- Integration tests that exercise it indirectly

Read all existing test cases to understand what's already covered.

### 3. Analyze the method for blind spots

For the target method, identify categories of blind spots:

**A. Input edge cases**
- Empty strings, null, undefined, zero, negative numbers
- Minimum and maximum valid values (boundary conditions)
- Values just outside valid ranges
- Special characters or unusual formats
- Extremely long inputs (performance/overflow concerns)

**B. State edge cases**
- Empty collections, initialized but unused state
- Concurrent access or race conditions (if applicable)
- State transitions that skip expected steps

**C. Error paths**
- Exceptions that could be thrown but aren't tested
- Error conditions that return special values (null, error objects)
- Network timeouts or external service failures (for I/O methods)
- Permission/authorization failures

**D. Logic branches**
- Every `if/else` branch
- Every `switch/case` path including `default`
- Every loop (empty iteration, single item, multiple items)
- Early returns and exit conditions

**E. External dependencies**
- API calls that could fail
- Database queries that return no results
- File system operations (file not found, permission denied)
- Third-party library edge cases

**F. Time-sensitive operations**
- Operations that depend on current time/date
- Timeout conditions
- Retry logic

### 4. Cross-reference with existing tests

For each blind spot category, check if existing tests already cover it. Mark as:
- ❌ **Uncovered**: No test exists for this scenario
- ⚠️ **Partially covered**: Some but not all aspects tested
- ✅ **Covered**: Adequately tested

### 5. Prioritize blind spots

Not all blind spots are equally important. Prioritize by:

**High priority** (cover these first):
- Error paths that could cause crashes
- Input validation failures
- Security-sensitive operations
- Data corruption risks

**Medium priority**:
- Boundary conditions
- Edge cases in business logic
- External dependency failures

**Low priority**:
- Unusual but harmless inputs
- Performance edge cases (unless explicitly requested)
- Cosmetic issues

### 6. Generate test suggestions

For each high and medium priority uncovered blind spot, provide:

1. **The scenario** in plain English
2. **Why it matters** in one sentence
3. **Test code template** showing how to test it

Format:

```
## Blind Spots Found

### [MethodName]

#### ❌ [Scenario description]
> **Why:** [one-sentence rationale]

**Test:**
```[language]
[test code template]
```
```

Group similar scenarios together. Include the file path and line numbers for reference.

### 7. Deliver the analysis

Present findings in this order:
1. Brief summary of what the method does
2. Existing test coverage overview
3. Prioritized list of blind spots
4. Test code templates for each

End with a question: "Which of these would you like me to implement as actual tests?"

## Guidelines

- **Be pragmatic**: Focus on value, not completeness. A method might have 50 theoretical edge cases but only 5 worth testing.
- **Respect existing style**: Match the test style, framework, and patterns already used in the codebase.
- **Stay focused**: One method at a time. Don't expand scope to the entire class or module unless explicitly asked.
- **No false positives**: Only report blind spots that actually exist. Verify the code path is real.
- **Language-aware**: Adapt to the language's testing conventions (Jest, pytest, RSpec, etc.)
