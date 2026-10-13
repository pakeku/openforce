# Openforce

Openforce is an open-source business operating system. CRM is the first application written on it. The work is the platform underneath: data, metadata, workflow, a UI runtime, integrations, permissions, audit, and a way for a developer to ship a business application as configuration.

Salesforce is the existence proof that one system can run a company's records, process, and integrations. It is also priced so that plenty of teams rent less than they need. Openforce is a side project aimed at that same class of power, in the open, starting from the kernel.

This repository is the specification. No service, schema, or interface is implemented yet.

## The platform

```text
Experience         Web · Admin · Mobile · Dashboards · Reports · Builder
                        │
Runtime            Objects · Views · Forms · Search · Workflow · Automation
                        │
Kernel             Metadata · Permissions · Events · Jobs · API · Audit
                        │
Infrastructure     PostgreSQL · Redis · Object storage
```

The kernel is the part that has to be right. Objects, metadata, permissions, events, jobs, and the API live there. Screens, workflows, and dashboards are applications on top of it. Postgres, Redis, and object storage are the infrastructure, because they are enough to start.

## Twelve primitives

An application on Openforce is a combination of these, plus a version of all of them.

| Primitive | It answers |
| --- | --- |
| [Object](requirements/primitives/README.md#object) | What kinds of things exist? |
| [Field](requirements/primitives/README.md#field) | What is true of each one? |
| [Relationship](requirements/primitives/README.md#relationship) | How do they connect? |
| [View](requirements/primitives/README.md#view) | How are they browsed? |
| [Action](requirements/primitives/README.md#action) | What can happen to them? |
| [Event](requirements/primitives/README.md#event) | What just happened? |
| [Workflow](requirements/primitives/README.md#workflow) | What should happen next? |
| [Permission](requirements/primitives/README.md#permission) | Who is allowed to do it? |
| [Page](requirements/primitives/README.md#page) | How is it shown? |
| [Integration](requirements/primitives/README.md#integration) | What else does it talk to? |
| [Function](requirements/primitives/README.md#function) | Where does custom code go? |
| [Version](requirements/primitives/README.md#version) | Which revision is deployed? |

An Account is an object. A pipeline is a view. Lead routing is a workflow. A CRM ships as a strong default configuration of these primitives.

## Read this

| | |
| --- | --- |
| Why this exists, and what it is aiming at | [Vision](requirements/vision/README.md) |
| The map of every requirement | [Requirements](requirements/README.md) |
| The rules that keep the design coherent | [Principles](requirements/principles/README.md) |
| What gets built, in what order | [Roadmap](requirements/roadmap/README.md) |

## Status

Informational. The requirements are written so implementation can start against them. The [roadmap](requirements/roadmap/README.md) is the build order. The first milestone is small on purpose: define an object, then create, read, update, and delete its records.
