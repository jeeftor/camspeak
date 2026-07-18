package api

// openAPISpec is the OpenAPI 3.0 specification for the camspeak REST API.
// Served at /api/openapi.json and used by the Swagger UI at /swagger.
const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "camspeak API",
    "description": "Camera audio router — stream TTS and audio to IP camera speakers via Hikvision ISAPI, Reolink, go2rtc, or ONVIF RTSP backchannel.",
    "version": "1.0",
    "license": {
      "name": "MIT",
      "url": "https://github.com/jeeftor/camspeak/blob/master/LICENSE"
    }
  },
  "servers": [
    {"url": "/api", "description": "Relative to this server"}
  ],
  "tags": [
    {"name": "audio", "description": "Speak, play, beep, broadcast"},
    {"name": "vision", "description": "Snapshot, vision, describe"},
    {"name": "library", "description": "Preset management"},
    {"name": "config", "description": "Runtime configuration"},
    {"name": "mqtt", "description": "MQTT status and browser"},
    {"name": "system", "description": "Health, events, cameras"}
  ],
  "paths": {
    "/speak": {
      "post": {
        "tags": ["audio"],
        "summary": "Text-to-speech on a single camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/SpeakRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/StatusResponse"}}}},
          "503": {"description": "TTS not configured"}
        }
      }
    },
    "/broadcast": {
      "post": {
        "tags": ["audio"],
        "summary": "TTS or preset to all cameras simultaneously",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/BroadcastRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/play": {
      "post": {
        "tags": ["audio"],
        "summary": "Play a saved library preset on a camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/PlayRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"},
          "404": {"description": "Preset not found"}
        }
      }
    },
    "/play-url": {
      "post": {
        "tags": ["audio"],
        "summary": "Download audio from URL, transcode, and play on camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/PlayURLRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/beep": {
      "post": {
        "tags": ["audio"],
        "summary": "Play an 800 Hz test beep on a camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/CameraRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/stop": {
      "post": {
        "tags": ["audio"],
        "summary": "Stop audio playback on a specific camera or all cameras",
        "description": "If the request body contains a camera name, only that camera is stopped. If empty or omitted, all cameras are stopped.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "camera": {"type": "string", "description": "Camera name to stop. If omitted, stops all cameras."}
                }
              }
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "object", "properties": {"status": {"type": "string"}, "camera": {"type": "string"}}}}}},
          "404": {"description": "Camera not found"}
        }
      }
    },
    "/snapshot/{camera}": {
      "get": {
        "tags": ["vision"],
        "summary": "Fetch a JPEG snapshot from the camera",
        "parameters": [
          {"name": "camera", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "JPEG image", "content": {"image/jpeg": {"schema": {"type": "string", "format": "binary"}}}},
          "502": {"description": "Frigate not reachable"}
        }
      }
    },
    "/vision": {
      "post": {
        "tags": ["vision"],
        "summary": "Snapshot to vision model, returns text description only (no TTS)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionResponse"}}}},
          "503": {"description": "Vision model not configured"}
        }
      }
    },
    "/vision/test": {
      "post": {
        "tags": ["vision"],
        "summary": "Capture snapshot (or reuse provided image) and run a vision prompt — for prompt testing/refinement",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionTestRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionTestResponse"}}}},
          "503": {"description": "Vision model or Frigate not configured"}
        }
      }
    },
    "/describe": {
      "post": {
        "tags": ["vision"],
        "summary": "Snapshot to vision model to TTS to speak on camera",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/DescribeRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DescribeResponse"}}}},
          "503": {"description": "Vision or TTS not configured"}
        }
      }
    },
    "/cameras": {
      "get": {
        "tags": ["system"],
        "summary": "List all cameras with online status",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Camera"}}}}}
        }
      }
    },
    "/voices": {
      "get": {
        "tags": ["system"],
        "summary": "List available TTS voices",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"type": "string"}}}}}
        }
      }
    },
    "/library": {
      "get": {
        "tags": ["library"],
        "summary": "List all saved audio presets",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Preset"}}}}}
        }
      },
      "post": {
        "tags": ["library"],
        "summary": "Generate a TTS clip and save as a preset",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/GeneratePresetRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Preset"}}}}
        }
      }
    },
    "/library/upload": {
      "post": {
        "tags": ["library"],
        "summary": "Upload an audio file (any format, ffmpeg transcodes to G.711)",
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "properties": {
                  "name": {"type": "string"},
                  "category": {"type": "string"},
                  "file": {"type": "string", "format": "binary"}
                },
                "required": ["name", "file"]
              }
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Preset"}}}}
        }
      }
    },
    "/library/{category}/{name}": {
      "delete": {
        "tags": ["library"],
        "summary": "Delete a library preset",
        "parameters": [
          {"name": "category", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK"}
        }
      },
      "patch": {
        "tags": ["library"],
        "summary": "Rename a preset (change name and/or category)",
        "parameters": [
          {"name": "category", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}, "category": {"type": "string"}}}}}},
        "responses": {
          "200": {"description": "Updated preset"},
          "409": {"description": "Target name already exists"}
        }
      }
    },
    "/library/{category}/{name}/preview": {
      "get": {
        "tags": ["library"],
        "summary": "Stream the audio for a preset (for in-browser playback)",
        "parameters": [
          {"name": "category", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "Audio stream", "content": {"audio/*": {"schema": {"type": "string", "format": "binary"}}}}
        }
      }
    },
    "/tts/preview": {
      "post": {
        "tags": ["library"],
        "summary": "Generate a TTS preview (audio blob, not saved)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/TTSPreviewRequest"}
            }
          }
        },
        "responses": {
          "200": {"description": "Audio blob", "content": {"audio/wav": {"schema": {"type": "string", "format": "binary"}}}}
        }
      }
    },
    "/events": {
      "get": {
        "tags": ["system"],
        "summary": "Server-Sent Events stream of speak/play/beep/broadcast/describe/stop actions",
        "responses": {
          "200": {"description": "SSE stream", "content": {"text/event-stream": {"schema": {"type": "string"}}}}
        }
      }
    },
    "/health": {
      "get": {
        "tags": ["system"],
        "summary": "Health check with version",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/HealthResponse"}}}}
        }
      }
    },
    "/config": {
      "get": {
        "tags": ["config"],
        "summary": "Current runtime configuration",
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/config/vision": {
      "get": {
        "tags": ["config"],
        "summary": "Get vision endpoint config",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionConfig"}}}}
        }
      },
      "put": {
        "tags": ["config"],
        "summary": "Update vision endpoint config (rebuilds vision client at runtime)",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionConfig"}
            }
          }
        },
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionConfig"}}}}
        }
      }
    },
    "/config/vision-prompts": {
      "get": {
        "tags": ["config"],
        "summary": "List all saved vision prompt presets",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/VisionPromptPreset"}}}}}
        }
      },
      "post": {
        "tags": ["config"],
        "summary": "Create or update a vision prompt preset",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/VisionPromptPreset"}
            }
          }
        },
        "responses": {
          "201": {"description": "Created", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/VisionPromptPreset"}}}}
        }
      }
    },
    "/config/vision-prompts/{name}": {
      "delete": {
        "tags": ["config"],
        "summary": "Delete a vision prompt preset",
        "parameters": [
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK"}
        }
      }
    },
    "/config/tts": {
      "get": {
        "tags": ["config"],
        "summary": "List all TTS presets",
        "responses": {
          "200": {"description": "OK"}
        }
      },
      "post": {
        "tags": ["config"],
        "summary": "Create a TTS preset",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPreset"}}}},
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/tts/{name}": {
      "put": {
        "tags": ["config"],
        "summary": "Update a TTS preset",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPreset"}}}},
        "responses": {"200": {"description": "OK"}}
      },
      "delete": {
        "tags": ["config"],
        "summary": "Delete a TTS preset (not the active one)",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}, "409": {"description": "Cannot delete active preset"}}
      }
    },
    "/config/tts/{name}/activate": {
      "post": {
        "tags": ["config"],
        "summary": "Set a TTS preset as active",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/cameras": {
      "get": {
        "tags": ["config"],
        "summary": "List cameras from config",
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "tags": ["config"],
        "summary": "Add a camera",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/CameraConfig"}}}},
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/cameras/{name}/toggle": {
      "patch": {
        "tags": ["config"],
        "summary": "Toggle camera enabled/disabled",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/cameras/{name}": {
      "delete": {
        "tags": ["config"],
        "summary": "Remove a camera",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/rules": {
      "get": {
        "tags": ["config"],
        "summary": "List MQTT rules",
        "responses": {"200": {"description": "OK"}}
      },
      "post": {
        "tags": ["config"],
        "summary": "Create an MQTT rule",
        "requestBody": {"required": true, "content": {"application/json": {}}},
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/config/airplay": {
      "get": {
        "tags": ["config"],
        "summary": "Get AirPlay receiver configuration",
        "responses": {"200": {"description": "AirPlay config with enabled flag and base_port"}}
      },
      "put": {
        "tags": ["config"],
        "summary": "Update AirPlay receiver configuration (requires restart)",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"enabled": {"type": "boolean"}, "base_port": {"type": "integer"}}}}}},
        "responses": {"200": {"description": "Updated — restart required for changes to take effect"}}
      }
    },
    "/mqtt/status": {
      "get": {
        "tags": ["mqtt"],
        "summary": "MQTT connection status",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/mqtt/events": {
      "get": {
        "tags": ["mqtt"],
        "summary": "SSE stream of MQTT messages",
        "responses": {"200": {"description": "SSE stream"}}
      }
    },
    "/mqtt/topics": {
      "get": {
        "tags": ["mqtt"],
        "summary": "All topics seen by the broker since startup",
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/mqtt/subscribe": {
      "post": {
        "tags": ["mqtt"],
        "summary": "Dynamically subscribe to a topic at runtime",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"topic": {"type": "string"}}}}}},
        "responses": {"200": {"description": "OK"}}
      }
    }
  },
  "components": {
    "schemas": {
      "SpeakRequest": {
        "type": "object",
        "required": ["camera", "text"],
        "properties": {
          "camera": {"type": "string", "description": "Camera name", "example": "backyard"},
          "text": {"type": "string", "description": "Text to speak", "example": "Hello world"},
          "voice": {"type": "string", "description": "TTS voice (empty = default)", "example": "af_sky"},
          "gain": {"type": "number", "description": "Audio gain multiplier", "default": 3.0, "example": 3.0}
        }
      },
      "BroadcastRequest": {
        "type": "object",
        "properties": {
          "text": {"type": "string", "example": "Announcement text"},
          "voice": {"type": "string", "example": "af_sky"},
          "preset": {"type": "string", "description": "Preset name (alternative to text)"},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "PlayRequest": {
        "type": "object",
        "required": ["camera", "preset"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "preset": {"type": "string", "example": "person_detected"},
          "category": {"type": "string", "example": "alerts"},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "PlayURLRequest": {
        "type": "object",
        "required": ["camera", "url"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "url": {"type": "string", "example": "http://host/audio.wav"},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "CameraRequest": {
        "type": "object",
        "required": ["camera"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"}
        }
      },
      "VisionRequest": {
        "type": "object",
        "required": ["camera"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "prompt": {"type": "string", "description": "Vision prompt (empty = camera/global default)", "example": "How many people do you see?"}
        }
      },
      "VisionResponse": {
        "type": "object",
        "properties": {
          "description": {"type": "string", "example": "There are 2 people in the driveway."}
        }
      },
      "VisionTestRequest": {
        "type": "object",
        "properties": {
          "camera": {"type": "string", "description": "Required if image is empty (to capture snapshot)", "example": "backyard"},
          "prompt": {"type": "string", "description": "Vision prompt to test", "example": "Describe what you see in one or two sentences."},
          "image": {"type": "string", "description": "Base64 data URI of a cached image. If provided, skips snapshot capture and reuses this image.", "example": "data:image/jpeg;base64,/9j/4AAQ..."}
        }
      },
      "VisionTestResponse": {
        "type": "object",
        "properties": {
          "description": {"type": "string", "example": "A white minivan parked in a driveway."},
          "image": {"type": "string", "description": "Base64 data URI of the image used (for client-side caching and display)", "example": "data:image/jpeg;base64,/9j/4AAQ..."}
        }
      },
      "DescribeRequest": {
        "type": "object",
        "required": ["camera"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "prompt": {"type": "string", "example": "Describe what you see."},
          "gain": {"type": "number", "default": 3.0}
        }
      },
      "DescribeResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"},
          "description": {"type": "string", "example": "A car is parked in the driveway."},
          "image": {"type": "string", "description": "Base64 JPEG data URI"}
        }
      },
      "StatusResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"}
        }
      },
      "HealthResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"},
          "version": {"type": "string", "example": "v1.10.0"}
        }
      },
      "Camera": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "example": "backyard"},
          "type": {"type": "string", "example": "hikvision"},
          "online": {"type": "boolean", "example": true}
        }
      },
      "CameraConfig": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "type": {"type": "string", "enum": ["hikvision", "reolink", "go2rtc", "onvif"]},
          "ip": {"type": "string"},
          "user": {"type": "string"},
          "pass": {"type": "string"},
          "channel": {"type": "integer", "default": 1},
          "stream": {"type": "string"},
          "enabled": {"type": "boolean", "default": false},
          "vision_prompt": {"type": "string"}
        }
      },
      "Preset": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "example": "person_detected"},
          "category": {"type": "string", "example": "alerts"},
          "duration": {"type": "number", "example": 1.4},
          "text": {"type": "string", "example": "Person detected"}
        }
      },
      "GeneratePresetRequest": {
        "type": "object",
        "required": ["name", "text"],
        "properties": {
          "name": {"type": "string", "example": "person_detected"},
          "text": {"type": "string", "example": "Person detected"},
          "category": {"type": "string", "default": "alerts"},
          "voice": {"type": "string", "example": "af_sky"}
        }
      },
      "TTSPreviewRequest": {
        "type": "object",
        "required": ["text"],
        "properties": {
          "text": {"type": "string", "example": "Hello world"},
          "voice": {"type": "string", "example": "af_sky"}
        }
      },
      "VisionConfig": {
        "type": "object",
        "properties": {
          "url": {"type": "string", "example": "http://192.168.1.91:8080/v1/chat/completions"},
          "model": {"type": "string", "example": "llama3.2-vision"},
          "api_key": {"type": "string"},
          "prompt": {"type": "string", "description": "Global default vision prompt"}
        }
      },
      "VisionPromptPreset": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": {"type": "string", "example": "concise-people"},
          "prompt": {"type": "string", "example": "Describe what you see in one or two sentences. Focus on people, vehicles, and animals."},
          "description": {"type": "string", "example": "Concise description focusing on people and vehicles"}
        }
      },
      "TTSPreset": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "endpoint": {"type": "string"},
          "model": {"type": "string"},
          "default_voice": {"type": "string"},
          "description": {"type": "string"}
        }
      }
    }
  }
}`
