import copy
import json
import unittest

from responses_history_text_probe import project


def fixture():
    return {'model': 'gpt-6-astra', 'stream': True, 'store': False, 'reasoning': {'effort': 'xhigh'},
            'input': [
                {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': 'Inspect the result.'}]},
                {'type': 'reasoning', 'id': 'rs_1', 'summary': [{'type': 'summary_text', 'text': 'Preserve the failed result.'}], 'encrypted_content': 'opaque'},
                {'type': 'function_call', 'id': 'fc_1', 'name': 'probe', 'call_id': 'call_1', 'arguments': '{"n":9007199254740993}'},
                {'type': 'function_call_output', 'call_id': 'call_1', 'output': [{'type': 'input_text', 'text': 'exit=1; failed'}, {'type': 'input_image', 'image_url': 'data:image/png;base64,AAAA'}]},
                {'type': 'message', 'role': 'assistant', 'content': [{'type': 'output_text', 'text': 'The operation failed.'}]},
                {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': 'Continue.'}]},
            ]}


class ProjectionTests(unittest.TestCase):
    def test_records_assets_and_failure_survive(self):
        body = fixture()
        original = copy.deepcopy(body)
        result, report = project(body)
        self.assertEqual(body, original)
        self.assertEqual(result['reasoning'], original['reasoning'])
        self.assertTrue(report['assets_hash_unchanged'])
        self.assertEqual(report['native_tool_pairs'], 1)
        self.assertEqual(report['cipher_fields_omitted'], {'reasoning': 1})
        text = json.dumps(result, ensure_ascii=False)
        self.assertIn('9007199254740993', text)
        self.assertIn('exit=1; failed', text)
        self.assertIn('Preserve the failed result.', text)
        self.assertNotIn('"encrypted_content":', text)
        self.assertFalse(any(item.get('type') == 'function_call_output' for item in result['input']))

    def test_unpaired_output_is_rejected(self):
        body = fixture()
        del body['input'][2]
        with self.assertRaises(RuntimeError):
            project(body)

    def test_unclosed_tool_round_is_rejected(self):
        body = fixture()
        del body['input'][4]
        with self.assertRaises(RuntimeError):
            project(body)

    def test_opaque_compaction_is_rejected(self):
        body = fixture()
        body['input'].insert(0, {'type': 'compaction', 'encrypted_content': 'only-memory'})
        with self.assertRaises(RuntimeError):
            project(body)

    def test_projection_cannot_be_applied_twice(self):
        result, _ = project(fixture())
        with self.assertRaises(RuntimeError):
            project(result)

    def test_remote_response_reference_is_rejected(self):
        body = fixture()
        body['previous_response_id'] = 'resp_remote'
        with self.assertRaises(RuntimeError):
            project(body)

    def test_agent_without_visible_body_is_rejected(self):
        body = fixture()
        body['input'].insert(1, {'type': 'agent_message', 'content': [{'type': 'encrypted_content', 'encrypted_content': 'opaque-only'}]})
        with self.assertRaises(RuntimeError):
            project(body)


if __name__ == '__main__':
    unittest.main()
