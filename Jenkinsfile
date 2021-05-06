#!/usr/bin/env groovy
pipeline {
  agent any
  environment {
    CI = '1'
  }
  options {
    timeout(time: 1, unit: 'HOURS')
    timestamps()
    ansiColor('xterm')
  }
  stages {
    stage('Checkout') {
      steps {
        checkout([
          $class                           : 'GitSCM',
          branches                         : scm.branches,
          doGenerateSubmoduleConfigurations: scm.doGenerateSubmoduleConfigurations,
          extensions                       : scm.extensions + [[$class: 'CloneOption', noTags: false, reference: '', shallow: false]],
          submoduleCfg                     : [],
          userRemoteConfigs                : scm.userRemoteConfigs
        ])
      }
    }

    stage('unit-test') {
      steps {
        sh "make test"
        publishHTML(target: [allowMissing         : false,
                             alwaysLinkToLastBuild: true,
                             keepAll              : true,
                             reportDir            : 'cover-report',
                             reportFiles          : 'coverage.html',
                             reportName           : 'Test Coverage Report',
                             reportTitles         : 'Test Coverage Report'])
      }
    }
    stage('build') {
      steps {
        sh "make production"
      }
    }
    stage('artifacts') {
      steps {
        script {
          def GRAVITY_VERSION = sh(script: 'make --silent get-version', returnStdout: true).trim()
          archiveArtifacts artifacts: "build/${GRAVITY_VERSION}/*",
            allowEmptyArchive: false,
            fingerprint: true,
            onlyIfSuccessful: true
        }
      }
    } // end stage:artifacts
  } //end stages
}
