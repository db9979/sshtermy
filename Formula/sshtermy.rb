# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.226"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.226/sshtermy_darwin_arm64"
      sha256 "0ab703db2ffa48c936c1086e30fe95cf06dda3fffbf7a29c7d210e775edbc233"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.226/sshtermy_darwin_amd64"
      sha256 "f4598880bfe7c3043c5573ec7006283171f679e0dec0a449856c91a938f5fdea"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.226/sshtermy_linux_arm64"
      sha256 "48737a02bde6ae02fe6bfa0d52429658afc7971b1abb79e328da655ee6c68686"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.226/sshtermy_linux_amd64"
      sha256 "472ec5761e82634282e5a9278007823a0abcd97f0edf1a133fc6c4e997bed678"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
