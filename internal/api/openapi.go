package api

// openAPISpec is the OpenAPI 3.0 specification for the camspeak REST API.
// Served at /api/openapi.json and used by the Swagger UI at /swagger.
const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "camspeak API",
    "description": "Camera audio router — stream TTS and audio to IP camera speakers via Hikvision ISAPI, Reolink, go2rtc, or ONVIF RTSP backchannel. First-class Home Assistant integration available via HACS (https://github.com/jeeftor/camspeak-hacs).",
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
        "summary": "Play a saved library preset (audio clip or stream) on a camera",
        "description": "A stream preset starts a live stream; an audio clip sends its saved G.711 audio. Stream presets and looped clips require the camera's live_stream capability (currently Hikvision). Use category and preset together; name-only requests must identify a unique preset. Check can_pause in playback state before offering pause/resume.",
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
    "/play-stream": {
      "post": {
        "tags": ["audio"],
        "summary": "Stream live audio from a URL or playlist to a camera",
        "description": "Starts ffmpeg to read a live stream or playlist (.pls/.m3u) and send G.711 mu-law to the camera speaker. Requires live_stream capability (currently Hikvision). go2rtc, Reolink, and ONVIF continuous streaming is unsupported. Stop with POST /api/stop.",
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
        "description": "If the request body contains a camera name, only that camera is stopped. If empty or omitted, all cameras are stopped. Tears down the ffmpeg process, closes the camera speaker connection, and resets AirPlay. For a softer suspend that can be resumed in place, use POST /api/pause instead.",
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
    "/pause": {
      "post": {
        "tags": ["audio"],
        "summary": "Pause a live stream on a specific camera or all cameras",
        "description": "Suspends the ffmpeg transcoder for an active /api/play-stream session or a looped preset (/api/play with loop!=0) via SIGSTOP without tearing down the camera speaker connection. Playback position is preserved and can be resumed in place with POST /api/resume. Only affects streams and looped presets; finite TTS/play/beep sends are unaffected. If the camera is omitted, all active streams are paused.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "camera": {"type": "string", "description": "Camera name to pause. If omitted, pauses all active streams."}
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string", "description": "paused, already-paused, or (when omitting camera) paused with a cameras array"},
                    "camera": {"type": "string"},
                    "cameras": {"type": "array", "items": {"type": "string"}}
                  }
                }
              }
            }
          },
          "404": {"description": "No active stream for the named camera"}
        }
      }
    },
    "/resume": {
      "post": {
        "tags": ["audio"],
        "summary": "Resume a paused live stream on a specific camera or all cameras",
        "description": "Resumes a stream or looped preset previously paused with POST /api/pause by sending SIGCONT to the ffmpeg transcoder. Only affects streams started via /api/play-stream and looped presets started via /api/play with loop!=0. If the camera is omitted, all paused streams are resumed.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "camera": {"type": "string", "description": "Camera name to resume. If omitted, resumes all paused streams."}
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string", "description": "resumed, not-paused, or (when omitting camera) resumed with a cameras array"},
                    "camera": {"type": "string"},
                    "cameras": {"type": "array", "items": {"type": "string"}}
                  }
                }
              }
            }
          },
          "404": {"description": "No active stream for the named camera"}
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
        "description": "Synchronous compatibility endpoint: waits for the full operation, including playback. Prefer POST /describe/jobs and polling GET /describe/jobs/{id} behind a reverse proxy or WAN gateway.",
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
    "/describe/jobs": {
      "post": {
        "tags": ["vision"],
        "summary": "Start a background Describe operation",
        "description": "Returns immediately with a job to poll instead of holding a request open through vision, TTS, and playback. At most two Describe jobs run concurrently; each has a ten-minute deadline. POST /stop cancels the camera's current operation. Do not automatically retry this POST when its response is lost: the operation may already have started.",
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DescribeRequest"}}}
        },
        "responses": {
          "202": {"description": "Job accepted", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DescribeJob"}}}},
          "400": {"description": "Invalid request or camera configuration"},
          "404": {"description": "Camera not found"},
          "409": {"description": "Camera configuration changed while starting the operation"},
          "503": {"description": "Vision or TTS unavailable, worker limit reached, or server shutting down"}
        }
      }
    },
    "/describe/jobs/{id}": {
      "get": {
        "tags": ["vision"],
        "summary": "Read Describe progress and completed timings",
        "description": "Poll while status is running. Results accumulate as real stages finish; playing indicates audio has started being sent, not confirmation of physical speaker sound. Terminal jobs are kept in memory for up to ten minutes, with at most 32 retained results. Server restart or eviction makes a job unavailable; a missing result does not prove playback failed.",
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {"description": "Current job snapshot", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DescribeJob"}}}},
          "404": {"description": "Unknown, expired, or evicted job"}
        }
      }
    },
    "/cameras": {
      "get": {
        "tags": ["system"],
        "summary": "List enabled cameras with online status, saved gain, and supported operations",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Camera"}}}}}
        }
      }
    },
    "/playback": {
      "get": {
        "tags": ["audio"],
        "summary": "Get current playback state for all cameras",
        "description": "Returns a map of enabled camera name to playback state: preparing, playing, paused, or idle. Use can_pause to decide whether to offer pause/resume; finite playback and AirPlay cannot be paused through this API. Detail identifies the source, and timestamps describe playback timing.",
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "additionalProperties": {"$ref": "#/components/schemas/PlaybackState"}
                }
              }
            }
          }
        }
      }
    },
    "/cameras/{name}/info": {
      "get": {
        "tags": ["system"],
        "summary": "Query camera device info and streaming settings (ISAPI/ONVIF)",
        "description": "Queries the camera's vendor API (Hikvision ISAPI or ONVIF SOAP) and returns device info, video/audio encoder configuration, and network info. Read-only.",
        "parameters": [
          {"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/CameraInfo"}}}},
          "404": {"description": "Camera not found"},
          "502": {"description": "Camera unreachable or query failed"}
        }
      }
    },
    "/cameras/{name}/volume": {
      "put": {
        "tags": ["audio"],
        "summary": "Set and persist camera gain",
        "description": "Updates the saved camera gain and applies it to the next audio chunk where supported. Zero mutes; values outside 0 through 10 are rejected.",
        "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"type": "object", "properties": {
            "gain": {"type": "number", "minimum": 0, "maximum": 10}
          }}}}
        },
        "responses": {
          "200": {"description": "Saved", "content": {"application/json": {"schema": {"type": "object", "properties": {
            "camera": {"type": "string"}, "gain": {"type": "number", "minimum": 0, "maximum": 10}
          }}}}},
          "400": {"description": "Invalid JSON or gain outside 0 through 10"},
          "404": {"description": "Camera not found"}
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
        "summary": "Generate a TTS clip and save as a preset, or save a stream URL as a stream preset",
        "description": "When 'url' is provided in the request body, a stream preset is created (no TTS, no raw file). When 'text' is provided, a TTS clip is generated and saved as a raw audio file. Either 'text' or 'url' must be provided.",
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
        "description": "Accepts at most 64 MiB for the entire multipart request, including fields and multipart overhead. At most two uploads/transcodes run concurrently. Returns a job_id immediately; poll GET /api/library/upload/jobs/{job_id} until status is done before playing the returned preset. Status error indicates conversion or saving failed.",
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
          "202": {"description": "Accepted — transcoding started", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UploadJobAccepted"}}}},
          "413": {"description": "Multipart request exceeds 64 MiB"},
          "503": {"description": "Both upload slots are occupied or the server is shutting down; retry after an active upload finishes"}
        }
      }
    },
    "/library/upload/jobs/{id}": {
      "get": {
        "tags": ["library"],
        "summary": "Poll the status of an async upload/transcode job",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UploadJob"}}}},
          "404": {"description": "Job not found"}
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
        "summary": "Playback activity stream, including recent persisted history",
        "description": "Each data frame is an EventEntry. Stable id values allow reconnect deduplication. Only playback and controls are recorded, not vision-only tests or configuration requests. Replay metadata is optional for older entries.",
        "responses": {
          "200": {"description": "SSE stream", "content": {"text/event-stream": {"schema": {"type": "string"}}}}
        }
      }
    },
    "/events/log": {
      "get": {
        "tags": ["system"],
        "summary": "Query historical event log as JSON",
        "parameters": [
          {"name": "limit", "in": "query", "schema": {"type": "integer", "default": 100, "maximum": 1000}, "description": "Max events to return"},
          {"name": "camera", "in": "query", "schema": {"type": "string"}, "description": "Filter by camera name"}
        ],
        "responses": {
          "200": {"description": "Event log", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/EventEntry"}}}}}
        }
      }
    },
    "/stream-levels": {
      "get": {
        "tags": ["system"],
        "summary": "SSE stream of audio levels for active streams (~10fps)",
        "description": "Server-Sent Events stream. Each event is a JSON object mapping camera names to audio level values (0.0–1.0). Only cameras with active streams are included. When no streams are active, empty events ({}) are sent as keepalives.",
        "responses": {
          "200": {"description": "SSE stream", "content": {"text/event-stream": {"schema": {"type": "object", "additionalProperties": {"type": "number"}}}}}
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
        "description": "An omitted or empty api_key preserves the saved key. Set clear_api_key to true with an empty key to remove it. Environment overrides remain authoritative. Responses omit the secret and expose has_api_key.",
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
    "/config/tts/benchmark": {
      "post": {
        "tags": ["config"],
        "summary": "Test buffered WAV or streaming PCM from a saved TTS preset without camera playback",
        "description": "Uses saved endpoint credentials without activating the preset. First byte is response delivery, not first audible sound. Streaming previews assume signed 16-bit little-endian PCM at the supplied rate/channels. No automatic retry. 30-second upstream timeout and 8 MiB audio limit.",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {
          "type": "object", "required": ["preset", "mode", "text", "sample_rate", "channels"],
          "properties": {
            "preset": {"type": "string"}, "mode": {"type": "string", "enum": ["buffered", "streaming"]},
            "text": {"type": "string", "maxLength": 2000}, "voice": {"type": "string"},
            "sample_rate": {"type": "integer", "minimum": 8000, "maximum": 96000},
            "channels": {"type": "integer", "enum": [1, 2]}
          }
        }}}},
        "responses": {
          "200": {"description": "Measured response and local WAV preview", "content": {"application/json": {"schema": {
            "type": "object", "properties": {
              "mode": {"type": "string"}, "first_byte_ms": {"type": "integer"}, "total_ms": {"type": "integer"},
              "bytes": {"type": "integer"}, "duration": {"type": "number", "description": "PCM duration in seconds; zero for buffered WAV"},
              "audio": {"type": "string", "description": "WAV data URI; never auto-play"}
            }
          }}}},
          "400": {"description": "Invalid test parameters"}, "404": {"description": "Preset not found"}, "502": {"description": "Unsupported transport or upstream failure"}
        }
      }
    },
    "/config/tts": {
      "get": {
        "tags": ["config"],
        "summary": "List all TTS presets",
        "responses": {
          "200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPresetList"}}}}
        }
      },
      "post": {
        "tags": ["config"],
        "summary": "Create a TTS preset",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPreset"}}}},
        "responses": {"201": {"description": "Created", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TTSPreset"}}}}}
      }
    },
    "/config/tts/{name}": {
      "put": {
        "tags": ["config"],
        "summary": "Update a TTS preset",
        "description": "An omitted or empty api_key preserves the saved key. Set clear_api_key to true with an empty key to remove it. Editing or activating the active preset refreshes the runtime client; environment overrides still win.",
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
    "/config/airplay": {
      "get": {
        "tags": ["config"],
        "summary": "Get AirPlay receiver configuration",
        "responses": {"200": {"description": "AirPlay config with enabled flag, base_port, and model"}}
      },
      "put": {
        "tags": ["config"],
        "summary": "Update AirPlay receiver configuration (requires restart)",
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"enabled": {"type": "boolean"}, "base_port": {"type": "integer"}, "prime_silence_ms": {"type": "integer"}, "model": {"type": "string", "description": "Device model advertised over mDNS; controls the iOS AirPlay icon", "example": "RealityDevice14,1"}, "gain": {"type": "number", "default": 1.0, "description": "Digital gain applied to AirPlay audio before sending to camera"}}}}}},
        "responses": {"200": {"description": "Updated — restart required for changes to take effect"}}
      }
    }
  },
  "components": {
    "schemas": {
      "CameraInfo": {
        "type": "object",
        "description": "Vendor-neutral camera settings (device info, streaming config, network).",
        "properties": {
          "type": {"type": "string", "example": "hikvision"},
          "online": {"type": "boolean"},
          "device": {"$ref": "#/components/schemas/DeviceInfo"},
          "network": {"$ref": "#/components/schemas/NetworkInfo"},
          "streams": {"type": "array", "items": {"$ref": "#/components/schemas/StreamInfo"}},
          "errors": {"type": "array", "items": {"type": "string"}}
        }
      },
      "DeviceInfo": {
        "type": "object",
        "properties": {
          "manufacturer": {"type": "string", "example": "Hikvision"},
          "model": {"type": "string", "example": "DS-2CD2042WD-I"},
          "firmware": {"type": "string", "example": "V5.5.0"},
          "serial": {"type": "string"},
          "device_type": {"type": "string"},
          "hardware": {"type": "string"}
        }
      },
      "NetworkInfo": {
        "type": "object",
        "properties": {
          "ip": {"type": "string", "example": "192.168.1.100"},
          "mac": {"type": "string", "example": "aa:bb:cc:dd:ee:ff"},
          "gateway": {"type": "string"},
          "subnet": {"type": "string"},
          "dns": {"type": "string"}
        }
      },
      "StreamInfo": {
        "type": "object",
        "properties": {
          "channel": {"type": "integer", "example": 1},
          "name": {"type": "string", "example": "Camera 01"},
          "video": {"$ref": "#/components/schemas/VideoInfo"},
          "audio": {"$ref": "#/components/schemas/AudioInfo"}
        }
      },
      "VideoInfo": {
        "type": "object",
        "properties": {
          "codec": {"type": "string", "example": "H.264"},
          "resolution": {"type": "string", "example": "1920x1080"},
          "width": {"type": "integer", "example": 1920},
          "height": {"type": "integer", "example": 1080},
          "framerate": {"type": "integer", "description": "fps", "example": 25},
          "bitrate": {"type": "integer", "description": "kbps", "example": 4096},
          "bitrate_type": {"type": "string", "example": "VBR"},
          "gop": {"type": "integer", "example": 50},
          "profile": {"type": "string", "example": "main"}
        }
      },
      "AudioInfo": {
        "type": "object",
        "properties": {
          "codec": {"type": "string", "example": "G.711ulaw"},
          "sample_rate": {"type": "integer", "description": "Hz", "example": 8000},
          "bitrate": {"type": "integer", "description": "kbps", "example": 64},
          "channels": {"type": "integer", "example": 1}
        }
      },
      "SpeakRequest": {
        "type": "object",
        "required": ["camera", "text"],
        "properties": {
          "camera": {"type": "string", "description": "Camera name", "example": "backyard"},
          "text": {"type": "string", "description": "Text to speak", "example": "Hello world"},
          "voice": {"type": "string", "description": "TTS voice (empty = default)", "example": "af_sky"},
          "gain": {"$ref": "#/components/schemas/PlaybackGain"}
        }
      },
      "BroadcastRequest": {
        "type": "object",
        "properties": {
          "text": {"type": "string", "example": "Announcement text"},
          "voice": {"type": "string", "example": "af_sky"},
          "preset": {"type": "string", "description": "Preset name (alternative to text). Supply category when names are duplicated across categories."},
          "category": {"type": "string", "example": "alerts"},
          "gain": {"$ref": "#/components/schemas/PlaybackGain"},
          "loop": {"type": "integer", "default": 0, "description": "Loop count: -1 = infinite (pausable/resumable), 0 = no loop (default), N = play N+1 times"}
        }
      },
      "PlayRequest": {
        "type": "object",
        "required": ["camera", "preset"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "preset": {"type": "string", "example": "person_detected", "description": "Preset name. Name-only lookup succeeds only when unique; ambiguous names require category."},
          "category": {"type": "string", "example": "alerts", "description": "Category and preset name together identify the saved audio or stream."},
          "gain": {"$ref": "#/components/schemas/PlaybackGain"},
          "loop": {"type": "integer", "default": 0, "description": "Loop count: -1 = infinite, 0 = no loop (default), N = play N+1 times. Uses ffmpeg -stream_loop, so the loop can be paused/resumed/stopped like a live stream via /api/pause, /api/resume, /api/stop."}
        }
      },
      "PlayURLRequest": {
        "type": "object",
        "required": ["camera", "url"],
        "properties": {
          "camera": {"type": "string", "example": "backyard"},
          "url": {"type": "string", "example": "http://host/audio.wav"},
          "gain": {"$ref": "#/components/schemas/PlaybackGain"}
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
          "stream": {"type": "string", "description": "Optional snapshot stream override"},
          "prompt": {"type": "string", "example": "Describe what you see."},
          "gain": {"$ref": "#/components/schemas/PlaybackGain"}
        }
      },
      "DescribeResponse": {
        "type": "object",
        "properties": {
          "status": {"type": "string", "example": "ok"},
          "description": {"type": "string", "example": "A car is parked in the driveway."},
          "image": {"type": "string", "description": "Base64 JPEG data URI"},
          "timings": {"type": "object", "description": "Completed stage durations in milliseconds: snapshot_ms (snap_ms is a compatibility alias), vision_ms, tts_ms, transcode_ms, send_open_ms, and send_playback_ms", "additionalProperties": {"type": "integer", "format": "int64"}},
          "ttfs_ms": {"type": "integer", "format": "int64", "description": "Time to first audio sent, in milliseconds; not an acoustic speaker measurement"},
          "total_ms": {"type": "integer", "format": "int64", "description": "Total operation duration in milliseconds"}
        }
      },
      "DescribeJob": {
        "type": "object",
        "required": ["id", "camera", "status", "stage", "elapsed_ms", "result"],
        "properties": {
          "id": {"type": "string"},
          "camera": {"type": "string", "example": "backyard"},
          "status": {"type": "string", "enum": ["running", "done", "error", "canceled"]},
          "stage": {"type": "string", "enum": ["snapshot", "vision", "tts", "transcode", "connecting", "playing", "done", "error", "canceled"]},
          "elapsed_ms": {"type": "integer", "format": "int64", "description": "Elapsed time since the job started, frozen at completion"},
          "result": {"$ref": "#/components/schemas/DescribeResponse"},
          "error": {"type": "string", "description": "Failure or cancellation detail; partial results and completed timings remain available"}
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
      "EventEntry": {
        "type": "object",
        "properties": {
          "id": {"type": "integer", "format": "int64", "description": "Stable persisted event identifier"},
          "replay": {"type": "object", "description": "Optional public playback request. Broadcast results use individual target commands. Current configuration still applies when replayed; no authentication headers are stored.", "properties": {
            "method": {"type": "string", "enum": ["POST"]},
            "path": {"type": "string", "example": "/api/play"},
            "body": {"type": "object", "additionalProperties": true},
            "redacted": {"type": "boolean", "description": "URL credentials/query parameters were removed and may need to be supplied privately"}
          }},
          "camera": {"type": "string", "example": "backyard"},
          "action": {"type": "string", "example": "speak"},
          "text": {"type": "string", "example": "Hello world"},
          "voice": {"type": "string", "example": "af_sky"},
          "at": {"type": "string", "format": "date-time", "example": "2026-08-13T13:40:43Z"}
        }
      },
      "Camera": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "example": "backyard"},
          "type": {"type": "string", "example": "hikvision"},
          "ip": {"type": "string"},
          "online": {"type": "boolean", "example": true},
          "gain": {"type": "number", "minimum": 0, "maximum": 10, "description": "Saved camera gain; zero is mute"},
          "capabilities": {"$ref": "#/components/schemas/CameraCapabilities"},
          "vision_prompt": {"type": "string"},
          "vision_stream": {"type": "string"},
          "vision_width": {"type": "integer"},
          "snap_method": {"type": "string"},
          "note": {"type": "string"},
          "airplay_enabled": {"type": "boolean"},
          "airplay_name": {"type": "string"},
          "airplay_model": {"type": "string"},
          "sort_order": {"type": "integer"}
        }
      },
      "CameraCapabilities": {
        "type": "object",
        "required": ["speak", "snapshot", "live_stream", "airplay"],
        "properties": {
          "speak": {"type": "boolean", "description": "Backend supports finite audio playback"},
          "snapshot": {"type": "boolean", "description": "A snapshot capture path is configured"},
          "live_stream": {"type": "boolean", "description": "Backend supports continuous audio streaming"},
          "airplay": {"type": "boolean", "description": "Backend supports AirPlay audio streaming; enabling the receiver is separate"}
        }
      },
      "PlaybackGain": {
        "type": "number",
        "minimum": 0,
        "maximum": 10,
        "description": "Optional digital gain multiplier. Omit to use each target camera's saved gain; explicit zero mutes. Applies to this playback request.",
        "example": 3.0
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
          "gain": {"type": "number", "minimum": 0, "maximum": 10, "description": "Saved digital gain; zero mutes"},
          "vision_prompt": {"type": "string"}
        }
      },
      "Preset": {
        "type": "object",
        "properties": {
          "name": {"type": "string", "example": "person_detected"},
          "category": {"type": "string", "example": "alerts"},
          "duration": {"type": "number", "example": 1.4},
          "text": {"type": "string", "example": "Person detected"},
          "url": {"type": "string", "description": "Live stream URL (stream presets only; empty for audio presets)", "example": "http://stream.example.com:8000/live"}
        }
      },
      "UploadJobAccepted": {
        "type": "object",
        "properties": {
          "job_id": {"type": "string", "example": "20260813-114407-1"},
          "status": {"type": "string", "example": "transcoding"},
          "name": {"type": "string", "example": "my_audio"},
          "category": {"type": "string", "example": "uploads"},
          "filename": {"type": "string", "example": "recording.mp3"}
        }
      },
      "UploadJob": {
        "type": "object",
        "properties": {
          "id": {"type": "string", "example": "20260813-114407-1"},
          "status": {"type": "string", "enum": ["transcoding", "saving", "done", "error"], "example": "transcoding"},
          "percent": {"type": "number", "description": "0–100, or -1 for indeterminate", "example": 45.2},
          "step": {"type": "string", "example": "Transcoding"},
          "name": {"type": "string", "example": "my_audio"},
          "category": {"type": "string", "example": "uploads"},
          "filename": {"type": "string", "example": "recording.mp3"},
          "error": {"type": "string", "description": "Present only when status is error"},
          "preset": {"$ref": "#/components/schemas/Preset"},
          "started_at": {"type": "string", "format": "date-time"},
          "done_at": {"type": "string", "format": "date-time"}
        }
      },
      "GeneratePresetRequest": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": {"type": "string", "example": "person_detected"},
          "text": {"type": "string", "description": "Text to synthesize (for TTS presets). Either text or url must be provided.", "example": "Person detected"},
          "url": {"type": "string", "description": "Live stream URL (for stream presets). Either text or url must be provided.", "example": "http://stream.example.com:8000/live"},
          "category": {"type": "string", "default": "alerts", "description": "Default: alerts for TTS, streams for stream presets"},
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
          "url": {"type": "string", "example": "http://10.0.0.x:8080/v1/chat/completions"},
          "model": {"type": "string", "example": "llama3.2-vision"},
          "api_key": {"type": "string", "writeOnly": true, "description": "Omitted or empty preserves the saved key"},
          "has_api_key": {"type": "boolean", "readOnly": true},
          "clear_api_key": {"type": "boolean", "writeOnly": true, "description": "With an empty api_key, explicitly removes the saved key"},
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
          "api_key": {"type": "string", "writeOnly": true, "description": "Omitted or empty preserves the saved key"},
          "has_api_key": {"type": "boolean", "readOnly": true},
          "clear_api_key": {"type": "boolean", "writeOnly": true},
          "is_active": {"type": "boolean"},
          "streaming": {"type": "boolean", "default": false, "description": "Opt-in Hikvision Speak/Describe PCM streaming"},
          "pcm_sample_rate": {"type": "integer", "default": 24000, "minimum": 8000, "maximum": 96000},
          "pcm_channels": {"type": "integer", "default": 1, "minimum": 1, "maximum": 2},
          "default_voice": {"type": "string"},
          "description": {"type": "string"}
        }
      },
      "PlaybackState": {
        "type": "object",
        "properties": {
          "state": {"type": "string", "enum": ["preparing", "playing", "paused", "idle"], "example": "playing"},
          "source": {"type": "string", "description": "Action or audio source, such as speak, play, play-url, stream, beep, describe, announce, or airplay", "example": "stream"},
          "can_pause": {"type": "boolean", "description": "Whether this operation supports the pause/resume endpoints"},
          "detail": {"type": "string", "example": "http://liveatc.net/stream.m3u"},
          "started_at": {"type": "string", "format": "date-time"},
          "paused_at": {"type": "string", "format": "date-time", "description": "Present only when state is paused"},
          "level": {"type": "number", "minimum": 0, "maximum": 1, "description": "Current audio level for VU meter (streams/loops only)", "example": 0.42}
        }
      },
      "TTSConfig": {
        "type": "object",
        "properties": {
          "streaming": {"type": "boolean", "default": false},
          "pcm_sample_rate": {"type": "integer"},
          "pcm_channels": {"type": "integer"},
          "url": {"type": "string"},
          "model": {"type": "string"},
          "default_voice": {"type": "string"},
          "has_api_key": {"type": "boolean", "readOnly": true}
        }
      },
      "TTSPresetList": {
        "type": "object",
        "properties": {
          "presets": {"type": "array", "items": {"$ref": "#/components/schemas/TTSPreset"}},
          "active": {"$ref": "#/components/schemas/TTSConfig"}
        }
      }
    }
  }
}`
