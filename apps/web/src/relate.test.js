import assert from "node:assert/strict"
import test from "node:test"
import { dedupeRelationships, isChildList, isLinkList, relationshipRequests, safeReturnPath } from "./relate.js"
import { objectRecordPath } from "./routes.js"

test("subobjects create the child field before the parent list", () => {
  const requests = relationshipRequests({
    kind: "subobjects",
    from: "todo",
    to: "comment",
    name: "comments",
    field: "todo",
    inverse: "",
  })
  assert.deepEqual(requests, [
    { object: "comment", body: { name: "todo", kind: "belongs_to", to: "todo", inverse: "comments" } },
    { object: "todo", body: { name: "comments", kind: "has_many", to: "comment", inverse: "todo" } },
  ])
})

test("a child field defaults to the parent object name", () => {
  const requests = relationshipRequests({
    kind: "subobjects",
    from: "account",
    to: "note",
    name: "notes",
    field: "",
    inverse: "",
  })
  assert.equal(requests[0].body.name, "account")
  assert.equal(requests[1].body.inverse, "account")
})

test("links and a single record stay on the object that declares them", () => {
  assert.deepEqual(relationshipRequests({
    kind: "links",
    from: "company",
    to: "tag",
    name: "tags",
    inverse: "companies",
    field: "",
  }), [
    { object: "company", body: { name: "tags", kind: "many_to_many", to: "tag", inverse: "companies" } },
  ])
  assert.deepEqual(relationshipRequests({
    kind: "one",
    from: "company",
    to: "contact",
    name: "owner",
    inverse: "",
    field: "",
  }), [
    { object: "company", body: { name: "owner", kind: "belongs_to", to: "contact" } },
  ])
})

test("child lists are subobjects, and a belongs_to field is the single record", () => {
  const fields = [{ name: "todo", type: "relation" }]
  assert.equal(isChildList({ name: "comments", kind: "has_many", to: "comment", inverse: "todo" }, fields), true)
  assert.equal(isChildList({ name: "comments", kind: "belongs_to", to: "comment", inverse: "todo" }, fields), true)
  assert.equal(isChildList({ name: "todo", kind: "belongs_to", to: "todo", inverse: "comments" }, fields), false)
  assert.equal(isLinkList({ kind: "many_to_many" }), true)
  assert.equal(isLinkList({ kind: "has_many" }), false)
})

test("duplicate relationship names keep the parent list", () => {
  const unique = dedupeRelationships([
    { name: "comments", kind: "belongs_to", to: "comment" },
    { name: "comments", kind: "has_many", to: "comment" },
    { name: "tags", kind: "many_to_many", to: "tag" },
  ])
  assert.deepEqual(unique.map((item) => [item.name, item.kind]), [
    ["comments", "has_many"],
    ["tags", "many_to_many"],
  ])
})

test("a new child record can return to its parent", () => {
  assert.equal(
    objectRecordPath("comment", undefined, { todo: "abc", back: "/objects/todo/parent" }),
    "/objects/comment/new?todo=abc&back=%2Fobjects%2Ftodo%2Fparent",
  )
  assert.equal(safeReturnPath("/objects/todo/parent"), "/objects/todo/parent")
  assert.equal(safeReturnPath("https://example.com"), "")
  assert.equal(safeReturnPath("//example.com"), "")
})
