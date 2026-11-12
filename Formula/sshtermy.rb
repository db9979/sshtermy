# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.366"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.366/sshtermy_darwin_arm64"
      sha256 "35c25f5ee1c6f01714815434adf9e40b8244c3628366572e766b478815f75847"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.366/sshtermy_darwin_amd64"
      sha256 "b3256995497f87fad91a97180c837661e5f14d939b0f606f5c12ea953b3a7dcc"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.366/sshtermy_linux_arm64"
      sha256 "a66cca640486c4112b9f9137c4aeef23c5495b6722ff87e840c61774f4ce605f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.366/sshtermy_linux_amd64"
      sha256 "a79c664adfdb6798270b28ce85f97bb280e552e76046550b12ecb2609817039b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
