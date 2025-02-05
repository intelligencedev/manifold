from transformers import AutoTokenizer, AutoModelForCausalLM
import torch
import soundfile as sf
from mlx_lm import load, generate

model, tokenizer = load("srinivasbilla/llasa-3b-Q4-mlx")

from xcodec2.modeling_xcodec2 import XCodec2Model
 
model_path = "srinivasbilla/xcodec2" 
 
Codec_model = XCodec2Model.from_pretrained(model_path)
Codec_model.eval().cpu()   

input_text = 'Hello, how are you doing today?'

def extract_speech_ids(speech_tokens_str):
 
    speech_ids = []
    for token_str in speech_tokens_str:
        if token_str.startswith('<|s_') and token_str.endswith('|>'):
            num_str = token_str[4:-2]

            num = int(num_str)
            speech_ids.append(num)
        else:
            print(f"Unexpected token: {token_str}")
    return speech_ids

#TTS start!
with torch.no_grad():
 
    formatted_text = f"<|TEXT_UNDERSTANDING_START|>{input_text}<|TEXT_UNDERSTANDING_END|>"

    # Tokenize the text
    chat = [
        {"role": "user", "content": "Convert the text to speech:" + formatted_text},
        {"role": "assistant", "content": "<|SPEECH_GENERATION_START|>"}
    ]

    input_ids = tokenizer.apply_chat_template(
        chat, tokenize=True, continue_final_message=True
    )

    print(input_ids)
    
    #input_ids = input_ids.to('mps')
    speech_end_id = tokenizer.convert_tokens_to_ids('<|SPEECH_GENERATION_END|>')

    # Generate the speech autoregressively
    outputs = generate(model, tokenizer, prompt=input_ids)


    # Remove <|SPEECH_GENERATION_START|> from the beginning of the outputs string
    outputs = outputs.replace('<|SPEECH_GENERATION_START|>', '')

    print(outputs)

    # example outputs:
    # <|s_25105|><|s_8984|><|s_25105|>

    # insert a comma between each token
    outputs = outputs.replace('><', '>,<')

    # Convert the string to a list of tokens
    speech_tokens = outputs.split(',')

    print(speech_tokens)

    # Loop through the strings and extract the numbers: <|s_25105|> -> 25105
    speech_ids = extract_speech_ids(speech_tokens)

    print(speech_ids)

    # Convert the strings to integers: ['25105', '8984', '25105'] -> [25105, 8984, 25105]
    speech_tokens = [int(x) for x in speech_ids]

    speech_tokens = torch.tensor(speech_tokens).cpu().unsqueeze(0).unsqueeze(0)

    # Decode the speech tokens to speech waveform
    gen_wav = Codec_model.decode_code(speech_tokens) 
 

sf.write("gen.wav", gen_wav[0, 0, :].cpu().numpy(), 16000)
