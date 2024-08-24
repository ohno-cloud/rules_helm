#!/usr/bin/env bash
bazel run @rules_go//go/tools/gopackagesdriver "${@}"
