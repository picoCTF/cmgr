#!/usr/bin/env python3
"""Generates metadata.json for the quiz: a static answer as the flag and a
seed-shuffled option list as a lookup value referenced by the details text."""

import json
import os
import random

options = ["Transport", "Network", "Session", "Data Link"]
answer = options[0]

random.seed(int(os.environ["SEED"]))
random.shuffle(options)

print(json.dumps({"flag": answer, "options": ", ".join(options)}))
