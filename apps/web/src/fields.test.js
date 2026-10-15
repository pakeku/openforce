import assert from "node:assert/strict"
import test from "node:test"
import { fieldDraft, fieldPayload, linkTargets } from "./fields.js"

test("a link field is a relation to an object", () => {
  assert.deepEqual(fieldPayload({
    name: " employer ",
    type: "link",
    required: true,
    to: "company",
    options: "nope",
  }), {
    name: "employer",
    type: "relation",
    required: true,
    to: "company",
  })
})

test("enum keeps choices and other types omit them", () => {
  assert.deepEqual(fieldPayload({ name: "industry", type: "enum", required: false, options: "SaaS, Finance, " }), {
    name: "industry",
    type: "enum",
    required: false,
    options: ["SaaS", "Finance"],
  })
  assert.deepEqual(fieldPayload({ name: "title", type: "text", required: false, to: "company", options: "x" }), {
    name: "title",
    type: "text",
    required: false,
  })
})

test("a saved relation opens as a link", () => {
  assert.deepEqual(fieldDraft({ name: "employer", type: "relation", required: false, to: "company" }), {
    name: "employer",
    type: "link",
    required: false,
    options: "",
    to: "company",
  })
})

test("the object being created can be the link target", () => {
  assert.deepEqual(linkTargets([{ name: "company", label: "Company" }], "person", "Person"), [
    { name: "person", label: "Person" },
    { name: "company", label: "Company" },
  ])
  assert.deepEqual(linkTargets([{ name: "person", label: "Person" }], "person", "Person"), [
    { name: "person", label: "Person" },
  ])
})
