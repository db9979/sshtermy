# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.312"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.312/sshtermy_darwin_arm64"
      sha256 "ece2fc179b6aded2cef6f78c987c35ef8243ad8a70bf0d754364be6eed952acf"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.312/sshtermy_darwin_amd64"
      sha256 "3dc7b179df6b9cfbef9f28fda97848e33de0a1cf74971518843131f2b3bb38b9"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.312/sshtermy_linux_arm64"
      sha256 "61f0fd33a90347f9070dc0cec79b6a8ecf59e5d2860ae305199ad3a9b159df81"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.312/sshtermy_linux_amd64"
      sha256 "3dca5d0b16888f183f54d99bd592293500d0747267c02f581cd0cbf37a154ff2"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
