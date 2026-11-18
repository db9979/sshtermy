# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.426"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.426/sshtermy_darwin_arm64"
      sha256 "0b54593050f6da1642836441713a1abf4e84287792d53aca259d82ad65026379"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.426/sshtermy_darwin_amd64"
      sha256 "d25b1df9c466824ec6ba089ec4c3504aa8e2d35a2938f70c81c8a1cf575481f3"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.426/sshtermy_linux_arm64"
      sha256 "d1e89ea6c40f065a6a2e34d816e23b8ceb813d9bf9cb221f5ef62b00e2c04498"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.426/sshtermy_linux_amd64"
      sha256 "f272147ab0360cf8dd168207fdc6942f3f23453aec2a809124ec70f1648cdbb3"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
