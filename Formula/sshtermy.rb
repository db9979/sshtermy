# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.420"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.420/sshtermy_darwin_arm64"
      sha256 "dbc5d960a8a7db173f53afecb71061d1ff195194b172995fe1e28c95b4a52128"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.420/sshtermy_darwin_amd64"
      sha256 "ea1747b1f24729d5dd871401c8b2680ed5bb7f122609e2fb08b94e3a1d4dbbe1"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.420/sshtermy_linux_arm64"
      sha256 "cb5c7eb71b7c570371857f0744dd84cd8117a65ef7b9d7a59f3c25f32f1df0f3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.420/sshtermy_linux_amd64"
      sha256 "e81519b010e4623302f03d3637bee918c7e09a4ec2c4ee5999840ab665afdbf8"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
