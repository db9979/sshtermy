# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.383"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.383/sshtermy_darwin_arm64"
      sha256 "8b446612ad257204036d0ffc973ebdbd775fd669d4b38e28127a014852953aa8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.383/sshtermy_darwin_amd64"
      sha256 "ce9dc15d36a4cae39301c2eaddd9936b0678ffb08920b919397ad1061b95a007"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.383/sshtermy_linux_arm64"
      sha256 "bd454f59b3cda31546cd567d2d1523d898b377a4d614f487eaf71c443a6c5495"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.383/sshtermy_linux_amd64"
      sha256 "efd709ba035eb098ae28b2b38d5595a162e17c9db27c78e47e635682f3a116a4"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
