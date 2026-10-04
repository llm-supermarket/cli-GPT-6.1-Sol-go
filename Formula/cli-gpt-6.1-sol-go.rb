class CliGpt61SolGo < Formula
  desc "Standalone cli-GPT-6.1-Sol-go CLI"
  homepage "https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go"
  version "0.1.1"

  on_macos do
    on_intel do
      url "https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go/releases/download/v0.1.1/cli-GPT-6.1-Sol-go_v0.1.1_darwin_amd64.tar.gz"
      sha256 "af20d598a80a84aef6d2c024cf064d7b2085879ca0f0a81d4810964ec5a3510d"
    end
    on_arm do
      url "https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go/releases/download/v0.1.1/cli-GPT-6.1-Sol-go_v0.1.1_darwin_arm64.tar.gz"
      sha256 "4afdca7045b7e9f91d5707321b9bb883f0320303dfbbcf42d218295d280a5c9a"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go/releases/download/v0.1.1/cli-GPT-6.1-Sol-go_v0.1.1_linux_amd64.tar.gz"
      sha256 "0c851606a00aa5b5f2082fbf4a5eaeae00ec5acffb1990eb7befacb280175c9f"
    end
    on_arm do
      url "https://github.com/llm-supermarket/cli-GPT-6.1-Sol-go/releases/download/v0.1.1/cli-GPT-6.1-Sol-go_v0.1.1_linux_arm64.tar.gz"
      sha256 "fca931affc3b5d6963dbdb31152e3d8461142ae5c31a9bfacf0abcc8f73b0676"
    end
  end

  def install
    bin.install "cli-GPT-6.1-Sol-go"
  end

  test do
    assert_predicate bin/"cli-GPT-6.1-Sol-go", :executable?
  end
end
