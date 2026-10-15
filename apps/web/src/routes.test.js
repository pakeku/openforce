import assert from "node:assert/strict"
import test from "node:test"
import { isWide, objectFieldsPath, objectRecordPath, objectTablePath, shellPath } from "./routes.js"

test("shell paths name the signed-in section", () => {
  assert.equal(shellPath("/home"), "home")
  assert.equal(shellPath("/objects/todo/abc"), "objects")
  assert.equal(shellPath("/flows"), "flows")
  assert.equal(shellPath("/approvals"), "approvals")
  assert.equal(shellPath("/notifications"), "notifications")
  assert.equal(shellPath("/missing"), "home")
})

test("flows is the wide page", () => {
  assert.equal(isWide("/flows"), true)
  assert.equal(isWide("/objects/todo"), false)
})

test("object urls keep one table and one record renderer", () => {
  assert.equal(objectTablePath("todo"), "/objects/todo")
  assert.equal(objectFieldsPath("todo"), "/objects/todo/fields")
  assert.equal(objectRecordPath("comment"), "/objects/comment/new")
  assert.equal(objectRecordPath("todo", "abc"), "/objects/todo/abc")
})
