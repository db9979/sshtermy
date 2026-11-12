# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.360"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.360/sshtermy_darwin_arm64"
      sha256 "aa504da0628b4671719af828092c06f9ea152bab8451feeb40256f00bc04a98e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.360/sshtermy_darwin_amd64"
      sha256 "5a14d3cdbf3fcca69ccbdece93d8e7b29978b77913483e5bd145b5242b3caa9e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.360/sshtermy_linux_arm64"
      sha256 "2564ba7484a47dbf201176018ef5551ff6a4006faea7cedd2f5c9afd777ea36b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.360/sshtermy_linux_amd64"
      sha256 "7313d8546e4410d5860d1ef28f530c1c4c18838dc2a44a1a13bde17745a1a69d"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
