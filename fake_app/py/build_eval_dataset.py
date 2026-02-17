#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
构造最终形态的评测数据集：基于「租房预约1.json」的结构，从初始评测集（评测案例_数据驱动_可支撑场景.json）
调 fake_app 接口拉取每轮期望的 houses，输出带 response.houses 的完整评测数据。

规则：
- 成功预约轮次（expected_tool_calls 为空）：houses = []
- 更新租赁状态轮次（含 PUT /api/houses/{id}/status）：调用 PUT，响应 data 即为修改后的房源详情，houses = [data]
- 其余轮次：按 expected_tool_calls / expected_tool_inputs 调 GET /api/houses 或 by_community，用返回 items 填 houses

用法：
  1. 先启动 fake_app（并加载好房源数据，如 database_2000.json）
  2. 全量生成: python build_eval_dataset.py [--base-url ...] [--input 评测案例路径] [--output 输出路径]
  3. 仅补充指定案例（新增/更新时用）: python build_eval_dataset.py --case-ids EV-41,EV-42 或 --case-id-range EV-21:EV-30
     - 若 --output 已存在，会先读取再合并（只更新传入的案例，其余保留）；若不存在则只写出这些案例。
"""

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Optional

try:
    import requests
except ImportError:
    print("请安装 requests: pip install requests", file=sys.stderr)
    sys.exit(1)


def _query_to_http_params(q: dict) -> dict:
    """把 expected_tool_inputs 中的 query 转为 GET 请求参数字典（标量保持，列表转逗号分隔）。"""
    if not q:
        return {}
    out = {}
    for k, v in q.items():
        if v is None:
            continue
        if isinstance(v, list):
            out[k] = ",".join(str(x) for x in v)
        elif isinstance(v, bool):
            out[k] = "true" if v else "false"
        else:
            out[k] = v
    return out


def fetch_houses_list(base_url: str, headers: dict, query: dict) -> list:
    """GET /api/houses，返回 data.items。"""
    params = _query_to_http_params(query)
    params.setdefault("page_size", 100)
    r = requests.get(f"{base_url.rstrip('/')}/api/houses", params=params, headers=headers, timeout=30)
    r.raise_for_status()
    data = r.json()
    if data.get("code") != 0:
        raise RuntimeError(f"API 返回 code={data.get('code')} msg={data.get('message')}")
    return data.get("data", {}).get("items", [])


def fetch_houses_by_community(base_url: str, headers: dict, community: str) -> list:
    """GET /api/houses/by_community，返回 data.items。"""
    r = requests.get(
        f"{base_url.rstrip('/')}/api/houses/by_community",
        params={"community": community},
        headers=headers,
        timeout=30,
    )
    r.raise_for_status()
    data = r.json()
    if data.get("code") != 0:
        raise RuntimeError(f"API 返回 code={data.get('code')} msg={data.get('message')}")
    return data.get("data", {}).get("items", [])


def fetch_house_by_id(base_url: str, headers: dict, house_id: str) -> dict:
    """GET /api/houses/{id}，返回 data（单套房源对象）。"""
    r = requests.get(f"{base_url.rstrip('/')}/api/houses/{house_id}", headers=headers, timeout=30)
    r.raise_for_status()
    data = r.json()
    if data.get("code") != 0:
        raise RuntimeError(f"API 返回 code={data.get('code')} msg={data.get('message')}")
    return data.get("data", {})


def put_house_status(base_url: str, headers: dict, house_id: str, status: str) -> dict:
    """PUT /api/houses/{id}/status，请求体 {"status": "rented"|"available"|"offline"}；响应 data 为修改后的房源完整对象。"""
    r = requests.put(
        f"{base_url.rstrip('/')}/api/houses/{house_id}/status",
        json={"status": status},
        headers={**headers, "Content-Type": "application/json"},
        timeout=30,
    )
    r.raise_for_status()
    data = r.json()
    if data.get("code") != 0:
        raise RuntimeError(f"API 返回 code={data.get('code')} msg={data.get('message')}")
    return data.get("data", {})


def get_round_houses(
    round_data: dict,
    base_url: str,
    headers: dict,
    prev_round_houses: list,
) -> list:
    """
    根据本轮的 expected_tool_calls / expected_tool_inputs（或兼容 query_params）调接口，返回该轮 houses。
    - 成功预约（expected_tool_calls 为空）：[]
    - 含 PUT /api/houses/{id}/status：从上下文或同轮 by_community 取 house_id，调 PUT，返回 [data]
    - 仅 PUT：house_id 必须来自上一轮 houses，上一轮为空则报错（案例需保证上一轮有房源返回）
    - 含 GET /api/houses：用 query 调列表，返回 items
    - 含 GET /api/houses/by_community：用 community 调，返回 items（若同轮还有 PUT 则本轮最终为 [PUT 的 data]）
    - 含 GET /api/houses/{id}：从上一轮取 house_id，调 GET，返回 [data]
    - 含 GET /api/houses/nearby_landmarks：[]（地标列表，不填 houses）
    """
    calls = round_data.get("expected_tool_calls") or []
    inputs = round_data.get("expected_tool_inputs") or []
    if not isinstance(calls, list):
        calls = []
    if not isinstance(inputs, list):
        inputs = []

    # 预约确认等无接口轮次
    if not calls:
        return []

    # 先解析出本轮可能用到的 query / community / body
    first_query = None
    community = None
    put_body_status = None
    for i, inp in enumerate(inputs):
        if not isinstance(inp, dict):
            continue
        if "query" in inp:
            q = inp["query"]
            if isinstance(q, dict) and "community" in q:
                community = q.get("community")
            first_query = first_query or q
        if "body" in inp and isinstance(inp["body"], dict):
            put_body_status = inp["body"].get("status")

    # 同轮同时 by_community + PUT：先 by_community 取首套房 id，再 PUT，返回 [PUT data]
    if "GET /api/houses/by_community" in calls and "PUT /api/houses/{id}/status" in calls and community and put_body_status:
        items = fetch_houses_by_community(base_url, headers, community)
        if not items:
            raise RuntimeError(f"by_community 未返回房源: community={community}")
        house_id = items[0].get("house_id") if isinstance(items[0], dict) else getattr(items[0], "house_id", None)
        if not house_id:
            raise RuntimeError("by_community 返回项无 house_id")
        house = put_house_status(base_url, headers, house_id, put_body_status)
        return [house] if house else []

    # 仅 PUT：house_id 来自上一轮第一套房
    if "PUT /api/houses/{id}/status" in calls and put_body_status:
        if not prev_round_houses:
            raise RuntimeError("PUT 轮次需要上一轮 houses 以获取 house_id")
        first_house = prev_round_houses[0]
        house_id = first_house.get("house_id") if isinstance(first_house, dict) else getattr(first_house, "house_id", None)
        if not house_id:
            raise RuntimeError("上一轮 houses 首项无 house_id")
        house = put_house_status(base_url, headers, house_id, put_body_status)
        return [house] if house else []

    # GET /api/houses/{id}：从上一轮取 house_id
    if "GET /api/houses/{id}" in calls:
        if not prev_round_houses:
            raise RuntimeError("GET /api/houses/{id} 需要上一轮 houses 以获取 house_id")
        first_house = prev_round_houses[0]
        house_id = first_house.get("house_id") if isinstance(first_house, dict) else getattr(first_house, "house_id", None)
        if not house_id:
            raise RuntimeError("上一轮 houses 首项无 house_id")
        house = fetch_house_by_id(base_url, headers, house_id)
        return [house] if house else []

    # 地标周边：不填 houses
    if "GET /api/houses/nearby_landmarks" in calls:
        return []

    # GET /api/houses/by_community
    if "GET /api/houses/by_community" in calls and community:
        return fetch_houses_by_community(base_url, headers, community)

    # GET /api/houses（列表）
    if "GET /api/houses" in calls and first_query and not community:
        return fetch_houses_list(base_url, headers, first_query)

    # 兼容旧格式：query_params / target_community（评测集关键字 → HTTP 参数名）
    _legacy_map = {"districts": "district", "areas": "area", "min_area_sqm": "min_area", "max_area_sqm": "max_area", "max_subway_distance": "max_subway_dist"}
    query_params = round_data.get("query_params") or {}
    if isinstance(query_params, dict):
        target_community = round_data.get("target_community") or query_params.get("target_community")
        target_area = round_data.get("target_area_or_community") or query_params.get("target_area_or_community")
        if target_community:
            return fetch_houses_by_community(base_url, headers, target_community)
        if target_area and not target_community:
            return fetch_houses_list(base_url, headers, {"area": target_area})
        if query_params and "target_community" not in query_params and "target_area_or_community" not in query_params:
            legacy_params = {_legacy_map.get(k, k): v for k, v in query_params.items()}
            return fetch_houses_list(base_url, headers, legacy_params)

    return []


def build_round_output(
    round_data: dict,
    case_id: str,
    base_url: str,
    headers: dict,
    prev_round_houses: list,
) -> dict:
    """构建单轮输出：session_id, user_input, response: { message, houses }，并保留 expected_* 等。"""
    houses = get_round_houses(round_data, base_url, headers, prev_round_houses)
    out = {
        "session_id": case_id,
        "user_input": round_data["user_input"],
        "response": {"message": "", "houses": houses},
    }
    if round_data.get("expected_message_contains") is not None:
        out["expected_message_contains"] = round_data["expected_message_contains"]
    if round_data.get("expected_tool_calls") is not None:
        out["expected_tool_calls"] = round_data["expected_tool_calls"]
    if round_data.get("expected_tool_inputs") is not None:
        out["expected_tool_inputs"] = round_data["expected_tool_inputs"]
    if round_data.get("action") is not None:
        out["action"] = round_data["action"]
    if round_data.get("query_params") is not None:
        out["query_params"] = round_data["query_params"]
    return out


def parse_case_id_set(case_ids_arg: Optional[str], case_id_range_arg: Optional[str]) -> Optional[set]:
    """
    解析 --case-ids 与 --case-id-range，返回要处理的 case_id 集合；若都未传则返回 None（表示全量）。
    --case-ids: 逗号分隔，如 EV-21,EV-22,EV-40
    --case-id-range: 含首尾，如 EV-21:EV-30 表示 EV-21 到 EV-30（按数字部分解析）。
    """
    if not case_ids_arg and not case_id_range_arg:
        return None
    out: set[str] = set()
    if case_ids_arg:
        for s in case_ids_arg.split(","):
            s = s.strip()
            if s:
                out.add(s)
    if case_id_range_arg:
        s = case_id_range_arg.strip()
        if ":" in s:
            m = re.match(r"^(EV-)?(\d+)\s*:\s*(EV-)?(\d+)$", s, re.I)
            if not m:
                raise ValueError(f"无效的 --case-id-range，应为如 EV-21:EV-30 或 EV-01:EV-09: {case_id_range_arg}")
            prefix = "EV-"
            s_lo, s_hi = m.group(2), m.group(4)
            lo, hi = int(s_lo), int(s_hi)
            if lo > hi:
                lo, hi = hi, lo
            width = max(len(s_lo), len(s_hi))
            for n in range(lo, hi + 1):
                out.add(f"{prefix}{n:0{width}d}")
        else:
            out.add(s)
    return out if out else None


def main():
    parser = argparse.ArgumentParser(description="调 fake_app 接口补充 houses，生成最终形态评测数据集（基于租房预约1结构）")
    parser.add_argument("--base-url", default="http://localhost:8080", help="fake_app 的 base URL")
    parser.add_argument("--user-id", default="eval_user", help="请求头 X-User-ID")
    parser.add_argument("--input", default=None, help="初始评测集 JSON 路径")
    parser.add_argument("--output", default=None, help="输出 JSON 路径")
    parser.add_argument("--case-ids", default=None, help="仅处理这些案例，逗号分隔，如 EV-41,EV-42")
    parser.add_argument("--case-id-range", default=None, help="仅处理该范围内案例，如 EV-21:EV-30（含首尾）")
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent
    default_input = repo_root / "data" / "case_init.json"
    default_output = repo_root / "data" / "case_result.json"

    input_path = Path(args.input) if args.input else default_input
    output_path = Path(args.output) if args.output else default_output

    try:
        case_id_set = parse_case_id_set(args.case_ids, args.case_id_range)
    except ValueError as e:
        print(str(e), file=sys.stderr)
        sys.exit(1)

    if not input_path.exists():
        print(f"输入文件不存在: {input_path}", file=sys.stderr)
        sys.exit(1)

    with open(input_path, "r", encoding="utf-8") as f:
        suite = json.load(f)

    headers = {"X-User-ID": args.user_id}
    cases_in = suite.get("cases", [])
    if case_id_set is not None:
        cases_in = [c for c in cases_in if c.get("case_id", "") in case_id_set]
        if len(cases_in) == 0:
            print("未在输入中找到任何匹配的 case_id，请检查 --case-ids / --case-id-range", file=sys.stderr)
            sys.exit(1)
        print(f"仅处理 {len(cases_in)} 个案例: {sorted(c.get('case_id') for c in cases_in)}", file=sys.stderr)

    base_url = args.base_url
    cases_out = []

    for c in cases_in:
        case_id = c.get("case_id", "")
        rounds_out = []
        prev_round_houses = []
        for i, r in enumerate(c.get("rounds", [])):
            try:
                round_out = build_round_output(r, case_id, base_url, headers, prev_round_houses)
                rounds_out.append(round_out)
                prev_round_houses = round_out.get("response", {}).get("houses", [])
            except Exception as e:
                print(f"[{case_id}] round {i+1} 调接口失败: {e}", file=sys.stderr)
                rounds_out.append({
                    "session_id": case_id,
                    "user_input": r["user_input"],
                    "response": {"message": "", "houses": []},
                    "expected_message_contains": r.get("expected_message_contains"),
                    "expected_tool_calls": r.get("expected_tool_calls"),
                    "expected_tool_inputs": r.get("expected_tool_inputs"),
                    "action": r.get("action"),
                    "query_params": r.get("query_params"),
                    "_error": str(e),
                })
                prev_round_houses = []
        cases_out.append({
            "case_id": case_id,
            "scenario_type": c.get("scenario_type"),
            "description": c.get("description"),
            "rounds": rounds_out,
        })

    # 若指定了案例范围且输出文件已存在：合并进已有数据集（按 case_id 顺序）
    if case_id_set is not None and output_path.exists():
        with open(output_path, "r", encoding="utf-8") as f:
            existing = json.load(f)
        existing_cases = {x["case_id"]: x for x in existing.get("cases", [])}
        for co in cases_out:
            existing_cases[co["case_id"]] = co
        # 保持原有顺序：先按原有 cases 顺序，再按 case_id 数字排新增的
        seen_order = [c["case_id"] for c in existing.get("cases", [])]
        added = [cid for cid in sorted(existing_cases.keys()) if cid not in seen_order]
        case_id_order = [cid for cid in seen_order if cid in existing_cases] + added
        cases_out = [existing_cases[cid] for cid in case_id_order]
        result = {
            "test_suite": existing.get("test_suite", suite.get("test_suite", "租房Agent评测数据集（完整版·租房预约1结构）")),
            "version": existing.get("version", suite.get("version", "1.0.0")),
            "usage": existing.get("usage", "评测时用 expected_message_contains、expected_tool_calls 与 response 与 Agent 实际输出比对判分。"),
            "cases": cases_out,
        }
    else:
        result = {
            "test_suite": suite.get("test_suite", "租房Agent评测数据集（完整版·租房预约1结构）"),
            "version": suite.get("version", "1.0.0"),
            "usage": "评测时用 expected_message_contains、expected_tool_calls 与 response 与 Agent 实际输出比对判分。",
            "cases": cases_out,
        }

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)

    print(f"已写入: {output_path}（共 {len(cases_out)} 个案例）")


if __name__ == "__main__":
    main()
