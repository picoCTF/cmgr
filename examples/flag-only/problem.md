# Layer Cake

- Namespace: cmgr/examples
- Type: flag-only
- Category: General Skills
- Points: 10

## Description

TCP provides reliable, ordered delivery of a byte stream between two hosts.
Which layer of the OSI model does it belong to?

## Details

Submit the name of the layer as your answer.  Possible answers: {{lookup("options")}}

## Hints

- The OSI model has seven layers; TCP sits directly above IP.

## Solution Overview

This is a knowledge-check question with no service or downloads: the answer
itself is submitted as the flag.  The build generates `metadata.json` with the
static answer as the flag and a seed-shuffled list of candidate answers as a
lookup value, demonstrating how `flag-only` challenges can implement
multiple-choice questions whose option order varies per build.

## Learning Objective

By the end of this challenge, competitors should be able to relate common
protocols to their OSI model layers.

## Tags

- example

## Attributes

- organization: cmgr
