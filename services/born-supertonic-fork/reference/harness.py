"""Parity harness for the Born Supertonic fork.

Usage (with the scratchpad venv that has onnx + onnxruntime + numpy, and the
Supertonic models under $MODELS):

    python harness.py gen-dp-ref                # writes dp_ref.json (inputs+output)
    python harness.py dump-shapes <model.onnx>  # writes ort_shapes.json
    python harness.py dump-int64 <model.onnx>   # writes ort_int64_vals.json
    python harness.py diff-shapes born_trace.txt ort_shapes.json
    python harness.py diff-values born_trace.txt ort_int64_vals.json

Born side: run the parity test with BORN_TRACE=1 and grep '^TRACE' into
born_trace.txt. See STATUS.md.
"""

import json
import sys

import numpy as np
import onnx
import onnxruntime as ort

MODELS = "models"  # dir with onnx/*.onnx + voice_styles/*.json


def _style_dp():
    d = json.load(open(f"{MODELS}/voice_styles/M1.json"))
    x = d["style_dp"]
    return np.array(x["data"], dtype=np.float32).reshape(x["dims"])


def gen_dp_ref():
    T = 8
    rng = np.random.default_rng(0)
    text_ids = rng.integers(1, 60, size=(1, T)).astype(np.int64)
    text_mask = np.ones((1, 1, T), dtype=np.float32)
    style_dp = _style_dp()
    sess = ort.InferenceSession(f"{MODELS}/onnx/duration_predictor.onnx")
    dur = np.array(sess.run(None, {"text_ids": text_ids, "style_dp": style_dp, "text_mask": text_mask})[0])
    json.dump({
        "text_ids": text_ids.flatten().tolist(), "text_ids_shape": list(text_ids.shape),
        "style_dp": style_dp.flatten().tolist(), "style_dp_shape": list(style_dp.shape),
        "text_mask": text_mask.flatten().tolist(), "text_mask_shape": list(text_mask.shape),
        "duration": dur.flatten().tolist(), "duration_shape": list(dur.shape),
    }, open("dp_ref.json", "w"))
    print("wrote dp_ref.json; duration =", dur.flatten().tolist())


def _run_all(model_path):
    m = onnx.load(model_path)
    names = [o for n in m.graph.node for o in n.output if o]
    for o in names:
        m.graph.output.extend([onnx.helper.ValueInfoProto(name=o)])
    ref = json.load(open("dp_ref.json"))
    feeds = {k: np.array(ref[k], dtype=(np.int64 if k == "text_ids" else np.float32)).reshape(ref[k + "_shape"])
             for k in ["text_ids", "style_dp", "text_mask"]}
    outs = ort.InferenceSession(m.SerializeToString()).run(names, feeds)
    return dict(zip(names, outs))


def dump_shapes(model_path):
    res = {n: list(np.array(v).shape) for n, v in _run_all(model_path).items()}
    json.dump(res, open("ort_shapes.json", "w"))
    print("dumped", len(res), "shapes")


def dump_int64(model_path):
    res = {}
    for n, v in _run_all(model_path).items():
        a = np.array(v)
        if a.dtype == np.int64 and a.size <= 64:
            res[n] = a.flatten().tolist()
    json.dump(res, open("ort_int64_vals.json", "w"))
    print("dumped", len(res), "int64 tensors")


def _born(trace):
    out = {}
    for line in open(trace):
        p = line.rstrip("\n").split("\t")
        if len(p) >= 3:
            out[p[1]] = p
    return out


def diff_shapes(trace, ref):
    ort_s = json.load(open(ref))
    for line in open(trace):
        p = line.rstrip("\n").split("\t")
        if len(p) < 3:
            continue
        name, shp = p[1], p[2]
        try:
            born = json.loads(shp.replace(" ", ","))
        except Exception:
            continue
        if name in ort_s and list(born) != list(ort_s[name]):
            print(f"FIRST SHAPE DIVERGENCE: {p[0]} {name}\n  born={born}\n  ort ={ort_s[name]}")
            return
    print("no shape divergence")


def diff_values(trace, ref):
    ort_v = json.load(open(ref))
    for line in open(trace):
        p = line.rstrip("\n").split("\t")
        if len(p) < 4 or not p[3].strip():
            continue
        try:
            born = json.loads(p[3])
        except Exception:
            continue
        if p[1] in ort_v and list(born) != list(ort_v[p[1]]):
            print(f"FIRST VALUE DIVERGENCE: {p[0]} {p[1]}\n  born={born}\n  ort ={ort_v[p[1]]}")
            return
    print("no value divergence")


if __name__ == "__main__":
    cmd = sys.argv[1]
    {"gen-dp-ref": lambda: gen_dp_ref(),
     "dump-shapes": lambda: dump_shapes(sys.argv[2]),
     "dump-int64": lambda: dump_int64(sys.argv[2]),
     "diff-shapes": lambda: diff_shapes(sys.argv[2], sys.argv[3]),
     "diff-values": lambda: diff_values(sys.argv[2], sys.argv[3])}[cmd]()
