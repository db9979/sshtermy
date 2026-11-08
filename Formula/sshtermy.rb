# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.238"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.238/sshtermy_darwin_arm64"
      sha256 "e4da2186ee0aeed60a44c0ac11386bc3cdb8a8c3a163b5856e069e6ad0e56f1a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.238/sshtermy_darwin_amd64"
      sha256 "37145db1ba701aa3c78439477cdc4eeda9f8957a8d825edb7fca82aa34df4efc"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.238/sshtermy_linux_arm64"
      sha256 "de8949568d7befc81c5e279cbae49c6e793e7e045d2e62158c47b79871468cd5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.238/sshtermy_linux_amd64"
      sha256 "7e3d3d1304d2b082e80b3f1a0878e5a3920e60c667e5ed92acc3dd9d4100a111"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
